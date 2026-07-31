package status

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/scaffold"
	"github.com/TheBud4/ray/internal/store"
)

// newTarget monta um projeto com ambiente ray mínimo e devolve o caminho.
func newTarget(t *testing.T, skills, agents, commands []string) string {
	t.Helper()
	target := t.TempDir()
	for dir, names := range map[string][]string{
		"skills": skills, "agents": agents, "commands": commands,
	} {
		for _, n := range names {
			p := filepath.Join(target, ".claude", dir, n)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return target
}

func TestRunReportsNoEnvironmentWhenClaudeDirMissing(t *testing.T) {
	rep, err := Run(nil, Options{Target: t.TempDir()}, Home{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(rep.Problems) != 1 {
		t.Fatalf("Problems = %v, want exactly one", rep.Problems)
	}
	if !strings.Contains(rep.Problems[0], "no ray environment") {
		t.Errorf("Problems[0] = %q, want it to name the missing environment", rep.Problems[0])
	}
	// Sem ambiente, nenhuma outra checagem deve ter rodado e inventado dado.
	if rep.Inventory != (Inventory{}) {
		t.Errorf("Inventory = %+v, want the zero value", rep.Inventory)
	}
}

func TestRunCountsInventory(t *testing.T) {
	target := newTarget(t, []string{"tdd/SKILL.md", "brainstorm/SKILL.md"},
		[]string{"reviewer.md"}, []string{"destilar.md", "revisar.md", "handoff.md"})

	rep, err := Run(nil, Options{Target: target}, Home{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := Inventory{Skills: 2, Agents: 1, Commands: 3}
	if rep.Inventory != want {
		t.Errorf("Inventory = %+v, want %+v", rep.Inventory, want)
	}
	if len(rep.Problems) != 0 {
		t.Errorf("Problems = %v, want none for a healthy environment", rep.Problems)
	}
}

// writeEnv monta receita + registro de perfil e devolve a Home.
func writeEnv(t *testing.T, target string, comps []profile.Component) Home {
	t.Helper()
	base := t.TempDir()
	home := Home{
		ProfilesDir: filepath.Join(base, "profiles"),
		StoreDir:    filepath.Join(base, "store"),
	}
	prof := &profile.Profile{Name: "test", Components: comps}
	if err := os.MkdirAll(home.ProfilesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := yaml.Marshal(prof)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home.ProfilesDir, "test.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(profile.ProfileRecordPath(target)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(profile.ProfileRecordPath(target), []byte("test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

// skillComponent é o componente que os testes de fork usam: via skills, que
// vendoriza em .claude/skills/<skill>.
func skillComponent() profile.Component {
	return profile.Component{Via: profile.ViaSkills, Skill: "tdd", Source: "o/r"}
}

// seedVendored escreve o conteúdo vendorizado do componente e devolve o hash
// da árvore como está no disco.
func seedVendored(t *testing.T, target, body string) string {
	t.Helper()
	dir := filepath.Join(target, ".claude", "skills", "tdd")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	h, err := store.HashTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func TestForksReportPristineWhenDiskMatchesBaseline(t *testing.T) {
	target := t.TempDir()
	home := writeEnv(t, target, []profile.Component{skillComponent()})
	h := seedVendored(t, target, "original\n")

	st := store.New(home.StoreDir)
	if err := st.SetPristine(target, "skills:o/r#tdd", h); err != nil {
		t.Fatal(err)
	}

	rep, err := Run(nil, Options{Target: target}, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(rep.Forks) != 1 || rep.Forks[0].State != ForkPristine {
		t.Errorf("Forks = %+v, want one ForkPristine", rep.Forks)
	}
}

func TestForksReportEditedWhenDiskDivergedFromBaseline(t *testing.T) {
	target := t.TempDir()
	home := writeEnv(t, target, []profile.Component{skillComponent()})
	seedVendored(t, target, "original\n")

	st := store.New(home.StoreDir)
	if err := st.SetPristine(target, "skills:o/r#tdd", "0000deadbeef"); err != nil {
		t.Fatal(err)
	}

	rep, err := Run(nil, Options{Target: target}, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(rep.Forks) != 1 || rep.Forks[0].State != ForkEdited {
		t.Errorf("Forks = %+v, want one ForkEdited", rep.Forks)
	}
}

// Sem linha-base o comando não pode afirmar nada offline: store.DecideOverwrite
// cairia no ramo que compara com o hash upstream, e com ele vazio responderia
// "não é fork" — errado, e errado na direção perigosa.
func TestForksReportUnknownWithoutPristineBaseline(t *testing.T) {
	target := t.TempDir()
	home := writeEnv(t, target, []profile.Component{skillComponent()})
	seedVendored(t, target, "original\n")

	rep, err := Run(nil, Options{Target: target}, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(rep.Forks) != 1 || rep.Forks[0].State != ForkUnknown {
		t.Errorf("Forks = %+v, want one ForkUnknown", rep.Forks)
	}
}

// gitFake devolve um FakeRunner que responde as duas consultas de git com as
// saídas dadas. A chave é runner.Command.String(), que ignora Dir.
func gitFake(lsFiles, porcelain string) *runner.FakeRunner {
	return &runner.FakeRunner{Results: map[string]runner.Result{
		"git ls-files -- .claude .mcp.json":           {Stdout: lsFiles},
		"git status --porcelain -- .claude .mcp.json": {Stdout: porcelain},
	}}
}

func TestGitNeverTrackedWhenLsFilesIsEmpty(t *testing.T) {
	target := newTarget(t, []string{"tdd/SKILL.md"}, nil, nil)

	rep, err := Run(gitFake("", ""), Options{Target: target}, Home{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if rep.Git != GitNeverTracked {
		t.Errorf("Git = %v, want GitNeverTracked", rep.Git)
	}
	// É o estado normal logo depois do init: nota, nunca problema.
	if len(rep.Problems) != 0 {
		t.Errorf("Problems = %v, want none — never-tracked is a note", rep.Problems)
	}
	if len(rep.AddPaths) == 0 {
		t.Error("AddPaths is empty; the note has no git add to offer")
	}
}

func TestGitDirtyWhenTrackedAndPorcelainHasOutput(t *testing.T) {
	target := newTarget(t, []string{"tdd/SKILL.md"}, nil, nil)

	rep, err := Run(gitFake(".claude/skills/tdd/SKILL.md\n", " M .claude/skills/tdd/SKILL.md\n?? .claude/agents/new.md\n"),
		Options{Target: target}, Home{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if rep.Git != GitDirty {
		t.Errorf("Git = %v, want GitDirty", rep.Git)
	}
	if rep.DirtyN != 2 {
		t.Errorf("DirtyN = %d, want 2", rep.DirtyN)
	}
}

func TestGitCleanWhenTrackedAndPorcelainEmpty(t *testing.T) {
	target := newTarget(t, []string{"tdd/SKILL.md"}, nil, nil)

	rep, err := Run(gitFake(".claude/skills/tdd/SKILL.md\n", ""), Options{Target: target}, Home{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if rep.Git != GitClean {
		t.Errorf("Git = %v, want GitClean", rep.Git)
	}
	if len(rep.Problems) != 0 {
		t.Errorf("Problems = %v, want none for a clean tree", rep.Problems)
	}
}

// Binário git ausente: erro do Run. A seção some e nada mais é afetado.
func TestGitUnavailableWhenCommandFails(t *testing.T) {
	target := newTarget(t, []string{"tdd/SKILL.md"}, nil, nil)
	failing := &runner.FakeRunner{Err: errors.New(`exec: "git": executable file not found in $PATH`)}

	rep, err := Run(failing, Options{Target: target}, Home{})
	if err != nil {
		t.Fatalf("Run() error = %v; a missing git must not fail the command", err)
	}
	if rep.Git != GitUnavailable {
		t.Errorf("Git = %v, want GitUnavailable", rep.Git)
	}
	if len(rep.Problems) != 0 {
		t.Errorf("Problems = %v, want none — an unavailable git is omitted, not a problem", rep.Problems)
	}
	if rep.Inventory.Skills != 1 {
		t.Errorf("Inventory.Skills = %d, want 1; the other checks must be unaffected", rep.Inventory.Skills)
	}
}

// Fora de um repositório: exit code ≠ 0, caminho diferente do binário ausente.
func TestGitUnavailableWhenNotARepository(t *testing.T) {
	target := newTarget(t, []string{"tdd/SKILL.md"}, nil, nil)
	notARepo := &runner.FakeRunner{Results: map[string]runner.Result{
		"git ls-files -- .claude .mcp.json": {ExitCode: 128, Stderr: "fatal: not a git repository"},
	}}

	rep, err := Run(notARepo, Options{Target: target}, Home{})
	if err != nil {
		t.Fatalf("Run() error = %v; outside a repo the command must still succeed", err)
	}
	if rep.Git != GitUnavailable {
		t.Errorf("Git = %v, want GitUnavailable", rep.Git)
	}
	if len(rep.Problems) != 0 {
		t.Errorf("Problems = %v, want none outside a git repository", rep.Problems)
	}
}

// writeGitignore grava um .gitignore com o bloco do ray, omitindo as linhas
// listadas em drop.
func writeGitignore(t *testing.T, target string, drop ...string) {
	t.Helper()
	begin, end := scaffold.GitignoreMarkers()
	lines := []string{begin}
	for _, l := range scaffold.GitignoreBaseLines() {
		if slices.Contains(drop, l) {
			continue
		}
		lines = append(lines, l)
	}
	lines = append(lines, end)
	body := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(target, ".gitignore"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestGitignoreIntactBlockIsNoProblem(t *testing.T) {
	target := newTarget(t, []string{"tdd/SKILL.md"}, nil, nil)
	writeGitignore(t, target)

	rep, err := Run(nil, Options{Target: target}, Home{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(rep.Problems) != 0 {
		t.Errorf("Problems = %v, want none for an intact block", rep.Problems)
	}
}

func TestGitignoreMissingNegationIsAProblemNamingIt(t *testing.T) {
	target := newTarget(t, []string{"tdd/SKILL.md"}, nil, nil)
	writeGitignore(t, target, "!.claude/skills/")

	rep, err := Run(nil, Options{Target: target}, Home{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(rep.Problems) != 1 {
		t.Fatalf("Problems = %v, want exactly one", rep.Problems)
	}
	if !strings.Contains(rep.Problems[0], "!.claude/skills/") {
		t.Errorf("Problems[0] = %q, want it to name the missing negation", rep.Problems[0])
	}
}

func TestGitignoreMissingBlockIsAProblem(t *testing.T) {
	target := newTarget(t, []string{"tdd/SKILL.md"}, nil, nil)
	if err := os.WriteFile(filepath.Join(target, ".gitignore"), []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rep, err := Run(nil, Options{Target: target}, Home{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(rep.Problems) != 1 || !strings.Contains(rep.Problems[0], "ray block") {
		t.Errorf("Problems = %v, want one naming the missing ray block", rep.Problems)
	}
}

// writeMCP grava um .mcp.json com um servidor de nome e comando dados.
func writeMCP(t *testing.T, target, name, command string) {
	t.Helper()
	body := fmt.Sprintf(`{"mcpServers":{%q:{"command":%q}}}`, name, command)
	if err := os.WriteFile(filepath.Join(target, ".mcp.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

type stubLooker map[string]bool

func (s stubLooker) Look(name string) bool { return s[name] }

func TestMCPServerWithCommandOnPathIsNoProblem(t *testing.T) {
	target := newTarget(t, []string{"tdd/SKILL.md"}, nil, nil)
	writeMCP(t, target, "brain", "npx")
	t.Setenv("RAY_BRAIN", "")

	rep, err := run(nil, stubLooker{"npx": true}, Options{Target: target}, Home{})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if len(rep.Problems) != 0 {
		t.Errorf("Problems = %v, want none", rep.Problems)
	}
	if rep.Inventory.MCPServers != 1 {
		t.Errorf("Inventory.MCPServers = %d, want 1", rep.Inventory.MCPServers)
	}
}

func TestMCPServerWithCommandOffPathIsAProblem(t *testing.T) {
	target := newTarget(t, []string{"tdd/SKILL.md"}, nil, nil)
	writeMCP(t, target, "brain", "nao-existe")
	t.Setenv("RAY_BRAIN", "")

	rep, err := run(nil, stubLooker{}, Options{Target: target}, Home{})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if len(rep.Problems) != 1 || !strings.Contains(rep.Problems[0], "nao-existe") {
		t.Errorf("Problems = %v, want one naming the missing command", rep.Problems)
	}
}

func TestMCPBrainPathThatDoesNotExistIsAProblem(t *testing.T) {
	target := newTarget(t, []string{"tdd/SKILL.md"}, nil, nil)
	t.Setenv("RAY_BRAIN", filepath.Join(t.TempDir(), "nao-existe"))

	rep, err := run(nil, stubLooker{}, Options{Target: target}, Home{})
	if err != nil {
		t.Fatalf("run() error = %v", err)
	}
	if len(rep.Problems) != 1 || !strings.Contains(rep.Problems[0], "RAY_BRAIN") {
		t.Errorf("Problems = %v, want one naming RAY_BRAIN", rep.Problems)
	}
}
