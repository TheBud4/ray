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

// O inventário conta conteúdo, não entradas de topo. Uma skill é um SKILL.md;
// README solto em skills/ e diretório sem SKILL.md não são skill, e agente que
// não é .md não é agente. Contar entradas de topo inflava os três números com
// qualquer arquivo que alguém deixasse ali.
func TestInventoryCountsContentNotTopLevelEntries(t *testing.T) {
	target := newTarget(t,
		[]string{"tdd/SKILL.md", "brainstorm/SKILL.md", "README.md"},
		[]string{"reviewer.md", "notas.txt"},
		[]string{"revisar.md"})
	if err := os.MkdirAll(filepath.Join(target, ".claude", "skills", "rascunho"), 0o755); err != nil {
		t.Fatal(err)
	}

	rep, err := Run(nil, Options{Target: target}, Home{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := Inventory{Skills: 2, Agents: 1, Commands: 1}
	if rep.Inventory != want {
		t.Errorf("Inventory = %+v, want %+v", rep.Inventory, want)
	}
}

// Comando com namespace mora em commands/<grupo>/<nome>.md e vira
// `/grupo:nome`. Contando entradas de topo, um grupo com três comandos contava
// 1 — o mesmo que um comando solto.
func TestInventoryCountsNamespacedCommands(t *testing.T) {
	target := newTarget(t, nil, nil,
		[]string{"revisar.md", "frontend/component.md", "frontend/layout.md"})

	rep, err := Run(nil, Options{Target: target}, Home{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if rep.Inventory.Commands != 3 {
		t.Errorf("Inventory.Commands = %d, want 3", rep.Inventory.Commands)
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

// writeEnvScaffold é o writeEnv com uma lista de arquivos de scaffold na
// receita. O `git add` da nota precisa conhecê-los: a whitelist do .gitignore
// só lista o que precisa de negação, e nada que o scaffold escreve na raiz
// (CLAUDE.md, SECURITY.md) é ignorado por alguém.
func writeEnvScaffold(t *testing.T, target string, scaffoldPaths []string) Home {
	t.Helper()
	home := writeEnv(t, target, nil)
	files := make([]profile.ScaffoldFile, len(scaffoldPaths))
	for i, p := range scaffoldPaths {
		files[i] = profile.ScaffoldFile{Path: p}
	}
	prof := &profile.Profile{Name: "test", Scaffold: profile.Scaffold{Files: files}}
	data, err := yaml.Marshal(prof)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home.ProfilesDir, "test.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	return home
}

// skillComponent é o componente que os testes de fork usam: cópia local que
// vendoriza em .claude/skills/tdd.
func skillComponent() profile.Component {
	return profile.Component{Name: "tdd", Dest: ".claude/skills"}
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
	if err := st.SetPristine(target, "tdd", h); err != nil {
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
	if err := st.SetPristine(target, "tdd", "0000deadbeef"); err != nil {
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

// Sem registro de perfil não há o que comparar, e isso é normal: um .claude/
// pode ter sido copiado à mão. O silêncio aqui é o que dá sentido ao aviso do
// teste seguinte.
func TestForksAreSilentWithoutARecordedProfile(t *testing.T) {
	target := newTarget(t, []string{"tdd/SKILL.md"}, nil, nil)

	rep, err := Run(nil, Options{Target: target}, Home{ProfilesDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(rep.Problems) != 0 {
		t.Errorf("Problems = %v, want none — a hand-copied .claude/ is normal", rep.Problems)
	}
	if len(rep.Forks) != 0 {
		t.Errorf("Forks = %+v, want none without a recipe to compare against", rep.Forks)
	}
}

// Receita registrada mas ilegível é outra coisa: o .ray-profile está lá, então
// o ray montou este ambiente, e mesmo assim a receita não carrega. Engolir esse
// erro junto com o caso normal deixa o usuário sem `profile:` na saída e sem
// nenhuma pista do porquê.
func TestForksReportProblemWhenRecordedProfileIsUnreadable(t *testing.T) {
	target := newTarget(t, []string{"tdd/SKILL.md"}, nil, nil)
	writeRayProfile(t, target)

	rep, err := Run(nil, Options{Target: target}, Home{ProfilesDir: t.TempDir()})
	if err != nil {
		t.Fatalf("Run() error = %v; an unreadable recipe is a finding, not a read failure", err)
	}
	if len(rep.Problems) != 1 {
		t.Fatalf("Problems = %v, want exactly one", rep.Problems)
	}
	if !strings.Contains(rep.Problems[0], "profile") {
		t.Errorf("Problems[0] = %q, want it to name the profile", rep.Problems[0])
	}
	if rep.Profile != "" {
		t.Errorf("Profile = %q, want empty when the recipe did not load", rep.Profile)
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

// O escopo do git tem que seguir a whitelist do .gitignore, não uma lista
// fixa: as duas dessincronizam em silêncio assim que a whitelist ganha uma
// entrada, e o status passa a não vigiar parte do ambiente que ele mesmo manda
// commitar. O denylist é a única exceção, e é explícita.
func TestGitScopeCoversEveryWhitelistedPath(t *testing.T) {
	scope := gitScope()
	for _, l := range scaffold.GitignoreBaseLines() {
		if !strings.HasPrefix(l, "!") || strings.Contains(l, "*") {
			continue
		}
		p := strings.TrimSuffix(strings.TrimPrefix(l, "!"), "/")
		if i := strings.IndexByte(p, '/'); i > 0 {
			p = p[:i]
		}
		if p == "" || gitScopeDenylist[p] {
			continue
		}
		if !slices.Contains(scope, p) {
			t.Errorf("gitScope() = %v, missing %q from the .gitignore whitelist", scope, p)
		}
	}
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

// O `git add` da nota tem que incluir o .gitignore. A whitelist não o conhece
// por construção — ela só lista o que precisa de negação, e o .gitignore não é
// ignorado por ninguém —, mas é o arquivo cujas negações fazem o conteúdo
// vendorizado ser commitado na máquina de quem clonar. Sem ele no add, o
// ambiente não viaja, que é a promessa inteira do produto.
func TestAddPathsIncludesTheGitignore(t *testing.T) {
	target := newTarget(t, []string{"tdd/SKILL.md"}, nil, nil)
	if err := os.WriteFile(filepath.Join(target, ".gitignore"), []byte("# >>> ray\n# <<< ray\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rep, err := Run(gitFake("", ""), Options{Target: target}, Home{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !slices.Contains(rep.AddPaths, ".gitignore") {
		t.Errorf("AddPaths = %v, want it to include .gitignore", rep.AddPaths)
	}
}

// Arquivo que a receita manda escrever e que ninguém ignora — CLAUDE.md,
// SECURITY.md — também é ambiente a commitar, e some da whitelist pelo mesmo
// motivo que o .gitignore.
func TestAddPathsIncludesScaffoldedFilesOutsideTheWhitelist(t *testing.T) {
	target := newTarget(t, []string{"tdd/SKILL.md"}, nil, nil)
	home := writeEnvScaffold(t, target, []string{"CLAUDE.md", "SECURITY.md", "docs/README.md"})
	for _, p := range []string{"CLAUDE.md", "SECURITY.md"} {
		if err := os.WriteFile(filepath.Join(target, p), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	rep, err := Run(gitFake("", ""), Options{Target: target}, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	for _, want := range []string{"CLAUDE.md", "SECURITY.md"} {
		if !slices.Contains(rep.AddPaths, want) {
			t.Errorf("AddPaths = %v, want it to include %q", rep.AddPaths, want)
		}
	}
}

// Caminho que a receita declara mas que não existe em disco não entra: o add
// falharia com pathspec inexistente, e a nota deixaria de ser copiável.
func TestAddPathsSkipsScaffoldedFilesNotOnDisk(t *testing.T) {
	target := newTarget(t, []string{"tdd/SKILL.md"}, nil, nil)
	home := writeEnvScaffold(t, target, []string{"CLAUDE.md", "SECURITY.md"})
	if err := os.WriteFile(filepath.Join(target, "CLAUDE.md"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rep, err := Run(gitFake("", ""), Options{Target: target}, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if slices.Contains(rep.AddPaths, "SECURITY.md") {
		t.Errorf("AddPaths = %v, want SECURITY.md out of it — it is not on disk", rep.AddPaths)
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

// writeRayProfile marca o projeto como montado pelo ray.
func writeRayProfile(t *testing.T, target string) {
	t.Helper()
	if err := os.WriteFile(profile.ProfileRecordPath(target), []byte("test\n"), 0o644); err != nil {
		t.Fatal(err)
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
	home := writeEnv(t, target, nil)
	writeGitignore(t, target, "!.claude/skills/")

	rep, err := Run(nil, Options{Target: target}, home)
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
	home := writeEnv(t, target, nil)
	if err := os.WriteFile(filepath.Join(target, ".gitignore"), []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rep, err := Run(nil, Options{Target: target}, home)
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

// Projeto que o ray não montou não tem .ray-profile, e o bloco do .gitignore
// nunca foi escrito por ele. Reclamar ali é julgar arquivo alheio — e produz
// falso positivo no próprio repositório do ray, cujo .claude/ é escrito à mão
// e commitado.
func TestGitignoreBlockIsNotCheckedWithoutARayProfile(t *testing.T) {
	target := newTarget(t, []string{"tdd/SKILL.md"}, nil, nil)
	if err := os.WriteFile(filepath.Join(target, ".gitignore"), []byte("node_modules/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rep, err := Run(nil, Options{Target: target}, Home{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(rep.Problems) != 0 {
		t.Errorf("Problems = %v, want none: ray did not scaffold this environment", rep.Problems)
	}
}
