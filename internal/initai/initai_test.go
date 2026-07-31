package initai

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/rayconfig"
	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/scaffold"
	"github.com/TheBud4/ray/internal/store"
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
		Integrations: profile.Integrations{Headroom: true, Brain: true, CodeGraph: true},
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

func TestRunWritesProfileRecord(t *testing.T) {
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

	data, err := os.ReadFile(filepath.Join(target, ".claude", ".ray-profile"))
	if err != nil {
		t.Fatalf("stat .claude/.ray-profile: %v", err)
	}
	if strings.TrimSpace(string(data)) != "test" {
		t.Errorf(".ray-profile = %q, want %q", data, "test")
	}
}

func TestRunWritesPristineHashForAcquiredComponent(t *testing.T) {
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

	st := store.New(home.StoreDir)
	onDiskHash, err := store.HashTree(filepath.Join(target, ".claude", "skills", "s"))
	if err != nil {
		t.Fatal(err)
	}
	pristine, ok := st.PristineHash(target, "skills:o/r#s")
	if !ok {
		t.Fatal("PristineHash() ok = false, want a pristine hash recorded after Run")
	}
	if pristine != onDiskHash {
		t.Errorf("PristineHash() = %q, want it to match the on-disk hash %q", pristine, onDiskHash)
	}
}

func TestRunLearnModeWritesLearnOverlay(t *testing.T) {
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
	if _, err := os.Stat(filepath.Join(target, ".claude/rules/learning-journal.md")); err != nil {
		t.Fatalf("stat learning-journal.md: %v", err)
	}
	// O prompt de ensino (escada de 4 degraus, contrato negociado) é parte do
	// overlay de learn desde 4822605, mas não tinha cobertura ponta-a-ponta a
	// partir do initai — só no pacote scaffold.
	if _, err := os.Stat(filepath.Join(target, ".claude/rules/learn-teaching.md")); err != nil {
		t.Fatalf("stat learn-teaching.md: %v", err)
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

func hasWarning(sum Summary, substr string) bool {
	for _, w := range sum.Warnings {
		if strings.Contains(w, substr) {
			return true
		}
	}
	return false
}

func mcpServerNames(t *testing.T, target string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(target, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Servers map[string]any `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(doc.Servers))
	for n := range doc.Servers {
		names = append(names, n)
	}
	return names
}

func TestRunWarnsWhenBrainUnconfigured(t *testing.T) {
	t.Setenv("RAY_BRAIN", "")
	home := newHome(t)
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()

	sum, err := Run(&seedingRunner{}, allFound, Options{
		Profile: "test", Target: target, Mode: scaffold.ModeBuild, Out: &bytes.Buffer{},
	}, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !hasWarning(sum, "brain ligado mas não configurado") {
		t.Errorf("Warnings = %v, want one about the unconfigured brain", sum.Warnings)
	}
	if slices.Contains(mcpServerNames(t, target), "brain") {
		t.Error("registered a brain MCP server with no path configured")
	}
}

// Caminho configurado mas inexistente vira aviso e NÃO registra o server:
// um MCP apontando para o vazio quebra em runtime, o que é pior que ausente.
func TestRunWarnsAndSkipsServerWhenBrainPathMissing(t *testing.T) {
	home := newHome(t)
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()

	ghost := filepath.Join(t.TempDir(), "nao-existe")
	t.Setenv("RAY_BRAIN", ghost)

	sum, err := Run(&seedingRunner{}, allFound, Options{
		Profile: "test", Target: target, Mode: scaffold.ModeBuild, Out: &bytes.Buffer{},
	}, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !hasWarning(sum, "does not exist") {
		t.Errorf("Warnings = %v, want one about the missing brain path", sum.Warnings)
	}
	if slices.Contains(mcpServerNames(t, target), "brain") {
		t.Error("registered a brain MCP server pointing at a nonexistent path")
	}
	// E o ray não pode ter criado o caminho para "consertar" a situação.
	if _, err := os.Stat(ghost); !os.IsNotExist(err) {
		t.Errorf("Run() created %s; ray is a consumer of the brain, not its owner", ghost)
	}
}

func TestRunRegistersBrainServerWhenPathValid(t *testing.T) {
	home := newHome(t)
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()

	brain := t.TempDir()
	t.Setenv("RAY_BRAIN", brain)

	sum, err := Run(&seedingRunner{}, allFound, Options{
		Profile: "test", Target: target, Mode: scaffold.ModeBuild, Out: &bytes.Buffer{},
	}, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if hasWarning(sum, "brain") {
		t.Errorf("Warnings = %v, want none for a valid brain path", sum.Warnings)
	}
	if !slices.Contains(mcpServerNames(t, target), "brain") {
		t.Errorf("MCP servers = %v, want a brain entry", mcpServerNames(t, target))
	}
}

func TestRunRecordsMCPJSONInCreated(t *testing.T) {
	home, target := newHome(t), t.TempDir()
	// testProfile() já liga Headroom/Brain/CodeGraph, então installer.Resolve
	// devolve servidores e mcp.WriteServers escreve o .mcp.json.
	writeProfile(t, home.ProfilesDir, testProfile())

	sum, err := Run(&seedingRunner{}, allFound,
		Options{Profile: "test", Target: target, Mode: scaffold.ModeBuild}, home)
	if err != nil {
		t.Fatal(err)
	}
	// Sem esta checagem o teste passaria por vacuidade se o perfil deixasse de
	// gerar servidor: nada seria exercitado.
	if len(mcpServerNames(t, target)) == 0 {
		t.Fatal("setup is wrong: no MCP server was written, the assertion below would be vacuous")
	}
	if !slices.Contains(sum.Created, ".mcp.json") {
		t.Errorf("Created = %v, want it to contain .mcp.json", sum.Created)
	}
}
