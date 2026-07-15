package initai

import (
	"bytes"
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
	}
}

func TestRunBuildModeFullFlow(t *testing.T) {
	home := newHome(t)
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()

	opts := Options{Profile: "test", Target: target, Mode: scaffold.ModeBuild, Out: &bytes.Buffer{}}
	sum, err := Run(&runner.FakeRunner{}, allFound, opts, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if sum.HadFailure {
		t.Fatalf("HadFailure = true, Failed = %v", sum.Failed)
	}

	for _, p := range []string{"CLAUDE.md", ".mcp.json", ".claude/settings.json", ".claude/hooks/session-start.sh"} {
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
	sum, err := Run(&runner.FakeRunner{}, allFound, opts, home)
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
	sum, err := Run(&runner.FakeRunner{}, allFound, opts, home)
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

	fr := &runner.FakeRunner{Results: map[string]runner.Result{
		"npx skills add o/r --skill s -a claude-code -y": {ExitCode: 1},
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
		if strings.Contains(f, "skills add o/r") {
			found = true
		}
	}
	if !found {
		t.Errorf("Failed = %v, want it to include the failed skills-add command", sum.Failed)
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

	fr := &runner.FakeRunner{}
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

	fr := &runner.FakeRunner{}
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
