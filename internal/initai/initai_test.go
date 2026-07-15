package initai

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/rayconfig"
	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/scaffold"
)

// seedingRunner é como runner.FakeRunner (registra chamadas, permite forçar
// Results por comando) mas também escreve em disco o efeito de um npx/git
// real — necessário porque o laço de aquisição do I2
// (acquire.CliAcquirer/GitAcquirer) inspeciona o disco depois de rodar o
// comando, não só o exit code.
type seedingRunner struct {
	Calls   []runner.Command
	Results map[string]runner.Result
}

func (s *seedingRunner) Run(_ context.Context, c runner.Command) (runner.Result, error) {
	s.Calls = append(s.Calls, c)
	if res, ok := s.Results[c.String()]; ok {
		return res, nil
	}
	if err := seedTestAcquisition(c); err != nil {
		return runner.Result{}, err
	}
	return runner.Result{ExitCode: 0}, nil
}

// seedTestAcquisition cobre só os padrões de comando que os fixtures deste
// arquivo produzem: o componente skills "s"/"o/r" de testProfile(), e um
// componente via:git de path "skills/widget" (TestRunAcquiresGitComponent).
func seedTestAcquisition(c runner.Command) error {
	switch {
	case c.Name == "npx" && len(c.Args) > 0 && c.Args[0] == "skills":
		skill := ""
		for i, a := range c.Args {
			if a == "--skill" && i+1 < len(c.Args) {
				skill = c.Args[i+1]
			}
		}
		if skill == "" {
			return nil
		}
		dir := filepath.Join(c.Dir, ".claude", "skills", skill)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+skill), 0o644)
	case c.Name == "git" && len(c.Args) > 0:
		clone := c.Args[len(c.Args)-1]
		dir := filepath.Join(clone, "skills", "widget")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# widget"), 0o644)
	}
	return nil
}

type stubLooker map[string]bool

func (s stubLooker) Look(name string) bool { return s[name] }

var allFound = stubLooker{
	"npx": true, "node": true, "python3.10+": true,
	"uv": true, "headroom": true, "graphify": true,
}

// testProfile is a minimal recipe exercising components, globals (headroom +
// code_graph), a per-project command (graphify update .) and one scaffold file.
func testProfile() *profile.Profile {
	return &profile.Profile{
		Name:         "test",
		Integrations: profile.Integrations{Headroom: true, KnowledgeVault: true, CodeGraph: true},
		Components:   []profile.Component{{Via: profile.ViaSkills, Skill: "s", Source: "o/r"}},
		Scaffold: profile.Scaffold{
			Files:    []profile.ScaffoldFile{{Path: "CLAUDE.md"}},
			Settings: map[string]any{"model": "opus"},
		},
	}
}

func writeProfile(t *testing.T, profilesDir string, p *profile.Profile) {
	t.Helper()
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := yaml.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, p.Name+".yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func newHome(t *testing.T) Home {
	t.Helper()
	base := t.TempDir()
	return Home{
		ProfilesDir:  filepath.Join(base, "profiles"),
		TemplatesDir: filepath.Join(base, "templates"),
		VaultDir:     filepath.Join(base, "vault"),
		ConfigPath:   filepath.Join(base, "config.yaml"),
		StatePath:    filepath.Join(base, "state.yaml"),
		StoreDir:     filepath.Join(base, "store"),
	}
}

func TestRunBuildModeFullFlow(t *testing.T) {
	home := newHome(t)
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()

	opts := Options{Profile: "test", Target: target, Mode: scaffold.ModeBuild, Out: &bytes.Buffer{}}
	sum, err := Run(&seedingRunner{}, allFound, opts, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if sum.HadFailure {
		t.Fatalf("HadFailure = true, Failed = %v", sum.Failed)
	}

	for _, p := range []string{"CLAUDE.md", ".mcp.json", ".claude/settings.json", ".claude/hooks/session-start.sh", ".claude/skills/s/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(target, p)); err != nil {
			t.Errorf("stat %s: %v", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(home.VaultDir, "README.md")); err != nil {
		t.Errorf("vault not ensured: %v", err)
	}
}

func TestRunWritesGitignore(t *testing.T) {
	home := newHome(t)
	p := testProfile()
	p.Scaffold.GitignoreStack = []string{"/{{.ProjectName}}"}
	writeProfile(t, home.ProfilesDir, p)
	target := t.TempDir()

	opts := Options{Profile: "test", Target: target, Mode: scaffold.ModeBuild, Out: &bytes.Buffer{}}
	sum, err := Run(&seedingRunner{}, allFound, opts, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if sum.HadFailure {
		t.Fatalf("HadFailure = true, Failed = %v", sum.Failed)
	}

	data, err := os.ReadFile(filepath.Join(target, ".gitignore"))
	if err != nil {
		t.Fatalf("stat .gitignore: %v", err)
	}
	content := string(data)
	for _, want := range []string{"!.claude/skills/", "graphify-out/", "/" + filepath.Base(target)} {
		if !strings.Contains(content, want) {
			t.Errorf(".gitignore = %q, want it to contain %q", content, want)
		}
	}

	found := false
	for _, c := range sum.Created {
		if c == ".gitignore" {
			found = true
		}
	}
	if !found {
		t.Errorf("Summary.Created = %v, want it to include .gitignore", sum.Created)
	}
}

func TestRunLearnModeOverlay(t *testing.T) {
	home := newHome(t)
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()

	opts := Options{Profile: "test", Target: target, Mode: scaffold.ModeLearn, Out: &bytes.Buffer{}}
	sum, err := Run(&seedingRunner{}, allFound, opts, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if sum.HadFailure {
		t.Fatalf("HadFailure = true, Failed = %v", sum.Failed)
	}

	for _, p := range []string{".claude/rules/learn.md", ".claude/hooks/guard-code.sh"} {
		if _, err := os.Stat(filepath.Join(target, p)); err != nil {
			t.Errorf("stat %s: %v", p, err)
		}
	}

	settingsData, err := os.ReadFile(filepath.Join(target, ".claude/settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(settingsData), "PreToolUse") {
		t.Errorf("settings.json = %s, want PreToolUse hook in learn mode", settingsData)
	}
}

func TestRunDryRunWritesNothing(t *testing.T) {
	home := newHome(t)
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()

	opts := Options{Profile: "test", Target: target, Mode: scaffold.ModeBuild, DryRun: true, Out: &bytes.Buffer{}}
	sum, err := Run(&runner.FakeRunner{}, allFound, opts, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	for _, p := range []string{"CLAUDE.md", ".mcp.json", ".claude/settings.json"} {
		if _, statErr := os.Stat(filepath.Join(target, p)); !os.IsNotExist(statErr) {
			t.Errorf("%s should not exist after dry-run, stat err = %v", p, statErr)
		}
	}
	if len(sum.Created) == 0 {
		t.Error("Summary.Created should still report what would be created in dry-run")
	}
}

func TestRunComponentFailureDoesNotAbort(t *testing.T) {
	home := newHome(t)
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()

	fr := &seedingRunner{Results: map[string]runner.Result{
		"npx skills add o/r --skill s -a claude-code -y --copy": {ExitCode: 1},
	}}
	opts := Options{Profile: "test", Target: target, Mode: scaffold.ModeBuild, Out: &bytes.Buffer{}}
	sum, err := Run(fr, allFound, opts, home)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil (component failure should not abort)", err)
	}
	if !sum.HadFailure {
		t.Fatal("HadFailure = false, want true")
	}
	found := false
	for _, f := range sum.Failed {
		if strings.Contains(f, "skills:o/r#s") {
			found = true
		}
	}
	if !found {
		t.Errorf("Failed = %v, want it to include the failed component coord", sum.Failed)
	}

	// Steps 8-10 still ran despite the isolated component failure.
	if _, err := os.Stat(filepath.Join(target, ".mcp.json")); err != nil {
		t.Errorf(".mcp.json missing after isolated component failure: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "CLAUDE.md")); err != nil {
		t.Errorf("CLAUDE.md missing after isolated component failure: %v", err)
	}
}

func TestRunPreflightAbortsBeforeAnyProjectEffect(t *testing.T) {
	home := newHome(t)
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()

	missingNpx := stubLooker{"node": true, "python3.10+": true, "uv": true}
	opts := Options{Profile: "test", Target: target, Mode: scaffold.ModeBuild, Out: &bytes.Buffer{}}

	_, err := Run(&runner.FakeRunner{}, missingNpx, opts, home)
	if err == nil {
		t.Fatal("Run() = nil error, want error when npx is missing")
	}
	if !strings.Contains(err.Error(), "ray doctor") {
		t.Errorf("error = %q, want it to hint at `ray doctor`", err.Error())
	}
	if _, err := os.Stat(filepath.Join(target, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Error("CLAUDE.md should not exist: preflight must abort before any project effect")
	}
}

func TestRunSkipsAlreadyInstalledGlobals(t *testing.T) {
	home := newHome(t)
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()

	st := &rayconfig.State{}
	st.AddGlobal("headroom")
	st.AddGlobal("code_graph")
	if err := st.Save(home.StatePath); err != nil {
		t.Fatal(err)
	}

	fr := &seedingRunner{}
	opts := Options{Profile: "test", Target: target, Mode: scaffold.ModeBuild, Out: &bytes.Buffer{}}
	sum, err := Run(fr, allFound, opts, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if sum.HadFailure {
		t.Fatalf("HadFailure = true, Failed = %v", sum.Failed)
	}

	for _, call := range fr.Calls {
		s := call.String()
		if strings.Contains(s, "headroom-ai") || strings.Contains(s, "install graphifyy") || strings.Contains(s, "install --platform claude") {
			t.Errorf("global install command ran despite already-installed state: %q", s)
		}
	}
	foundProjectCmd := false
	for _, call := range fr.Calls {
		if call.String() == "graphify update ." {
			foundProjectCmd = true
		}
	}
	if !foundProjectCmd {
		t.Error("expected the per-project `graphify update .` command to still run")
	}
}

func TestRunNoGlobalSkipsAllGlobalInstalls(t *testing.T) {
	home := newHome(t)
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()

	fr := &seedingRunner{}
	opts := Options{Profile: "test", Target: target, Mode: scaffold.ModeBuild, NoGlobal: true, Out: &bytes.Buffer{}}
	sum, err := Run(fr, allFound, opts, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if sum.HadFailure {
		t.Fatalf("HadFailure = true, Failed = %v", sum.Failed)
	}

	for _, call := range fr.Calls {
		s := call.String()
		if strings.Contains(s, "headroom-ai") || strings.Contains(s, "install graphifyy") || strings.Contains(s, "install --platform claude") {
			t.Errorf("global install command ran despite --no-global: %q", s)
		}
	}
}

func TestRunContentCacheFirstNoReacquireOnSecondRun(t *testing.T) {
	home := newHome(t)
	writeProfile(t, home.ProfilesDir, testProfile())
	target1 := t.TempDir()
	target2 := t.TempDir()
	sr := &seedingRunner{}

	opts1 := Options{Profile: "test", Target: target1, Mode: scaffold.ModeBuild, Out: &bytes.Buffer{}}
	if _, err := Run(sr, allFound, opts1, home); err != nil {
		t.Fatalf("Run() 1st run error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(target1, ".claude/skills/s/SKILL.md")); err != nil {
		t.Fatalf("1st run: content missing: %v", err)
	}

	opts2 := Options{Profile: "test", Target: target2, Mode: scaffold.ModeBuild, Out: &bytes.Buffer{}}
	if _, err := Run(sr, allFound, opts2, home); err != nil {
		t.Fatalf("Run() 2nd run error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(target2, ".claude/skills/s/SKILL.md")); err != nil {
		t.Fatalf("2nd run: content missing (should restore from the warm cache): %v", err)
	}

	acquireCalls := 0
	for _, c := range sr.Calls {
		if c.Name == "npx" && len(c.Args) > 0 && c.Args[0] == "skills" {
			acquireCalls++
		}
	}
	if acquireCalls != 1 {
		t.Fatalf("acquire calls = %d, want exactly 1: the 2nd run should hit the warm cache, not re-acquire", acquireCalls)
	}
}

func TestRunAcquiresGitComponent(t *testing.T) {
	home := newHome(t)
	p := testProfile()
	p.Components = append(p.Components, profile.Component{Via: profile.ViaGit, Repo: "acme/skills", Path: "skills/widget"})
	writeProfile(t, home.ProfilesDir, p)
	target := t.TempDir()

	opts := Options{Profile: "test", Target: target, Mode: scaffold.ModeBuild, Out: &bytes.Buffer{}}
	sum, err := Run(&seedingRunner{}, allFound, opts, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if sum.HadFailure {
		t.Fatalf("HadFailure = true, Failed = %v", sum.Failed)
	}
	if _, err := os.Stat(filepath.Join(target, ".claude/skills/widget/SKILL.md")); err != nil {
		t.Fatalf("git component content missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, ".claude/skills/widget/.ray-origin")); err != nil {
		t.Fatalf(".ray-origin missing: %v", err)
	}
}
