package initai

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/TheBud4/ray/internal/preflight"
	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/rayconfig"
	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/store"
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
		Integrations: profile.Integrations{Headroom: true, Brain: true, CodeGraph: true},
		Components:   []profile.Component{{Name: "s", Dest: ".claude/skills"}},
		Scaffold: profile.Scaffold{
			Files:    []profile.ScaffoldFile{{Path: "CLAUDE.md"}},
			Settings: map[string]any{"model": "opus"},
		},
	}
}

// seedComponent cria o conteúdo de origem de um componente em
// home.ComponentsDir/name — como se o usuário já tivesse colocado ali. O ray
// nunca baixa nada; sem isto, o componente falha por "not found", que é
// exatamente o comportamento que TestRunComponentFailureDoesNotAbort explora
// de propósito ao não chamar este helper.
func seedComponent(t *testing.T, home Home, name string) {
	t.Helper()
	dir := filepath.Join(home.ComponentsDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name), 0o644); err != nil {
		t.Fatal(err)
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
		ProfilesDir:   filepath.Join(base, "profiles"),
		TemplatesDir:  filepath.Join(base, "templates"),
		ConfigPath:    filepath.Join(base, "config.yaml"),
		StatePath:     filepath.Join(base, "state.yaml"),
		StoreDir:      filepath.Join(base, "store"),
		ComponentsDir: filepath.Join(base, "components"),
	}
}

func TestRunBuildModeFullFlow(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()

	opts := Options{Profile: "test", Target: target, Out: &bytes.Buffer{}}
	sum, err := Run(&runner.FakeRunner{}, allFound, opts, home)
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
	seedComponent(t, home, "s")
	p := testProfile()
	p.Scaffold.GitignoreStack = []string{"/{{.ProjectName}}"}
	writeProfile(t, home.ProfilesDir, p)
	target := t.TempDir()

	opts := Options{Profile: "test", Target: target, Out: &bytes.Buffer{}}
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

func TestRunDryRunWritesNothing(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()

	opts := Options{Profile: "test", Target: target, DryRun: true, Out: &bytes.Buffer{}}
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

// O alvo inexistente é o caso que o teste acima não cobre: com t.TempDir() o
// diretório já existe, o MkdirAll do ensureWritableDir é no-op e o probe é
// apagado — o vazamento fica invisível. É por aqui que ele aparece.
func TestRunDryRunDoesNotCreateTheTargetDir(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := filepath.Join(t.TempDir(), "ainda-nao-existe")

	opts := Options{Profile: "test", Target: target, DryRun: true, Out: &bytes.Buffer{}}
	if _, err := Run(&runner.FakeRunner{}, allFound, opts, home); err != nil {
		t.Fatalf("Run() error = %v", err)
	}

	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Errorf("stat(%s) err = %v, want IsNotExist — dry-run must not create the target", target, statErr)
	}
}

// Sem rede, a forma de forçar uma falha isolada de componente é simples:
// não semear a pasta de origem em home.ComponentsDir. "not found" é o único
// jeito de um componente falhar agora — não há mais exit code de instalador.
func TestRunComponentFailureDoesNotAbort(t *testing.T) {
	home := newHome(t)
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()

	opts := Options{Profile: "test", Target: target, Out: &bytes.Buffer{}}
	sum, err := Run(&runner.FakeRunner{}, allFound, opts, home)
	if err != nil {
		t.Fatalf("Run() error = %v, want nil (component failure should not abort)", err)
	}
	if !sum.HadFailure {
		t.Fatal("HadFailure = false, want true")
	}
	if !slices.Contains(sum.Failed, "s") {
		t.Errorf("Failed = %v, want it to include the failed component name", sum.Failed)
	}
	if !hasWarning(sum, "not found") {
		t.Errorf("Warnings = %v, want one naming the missing component path", sum.Warnings)
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
	opts := Options{Profile: "test", Target: target, Out: &bytes.Buffer{}}

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

// O gate tinha o Hint na mão e mandava o usuário descobri-lo noutro comando.
// Erro tipado, não comparação de string: quem consome quer os Checks.
func TestRunPreflightErrorCarriesTheHint(t *testing.T) {
	home := newHome(t)
	writeProfile(t, home.ProfilesDir, testProfile())

	missingNpx := stubLooker{"node": true, "python3.10+": true, "uv": true}
	opts := Options{Profile: "test", Target: t.TempDir(), Out: &bytes.Buffer{}}

	_, err := Run(&runner.FakeRunner{}, missingNpx, opts, home)

	var missing *preflight.MissingRequiredError
	if !errors.As(err, &missing) {
		t.Fatalf("Run() error = %v (%T), want a *preflight.MissingRequiredError", err, err)
	}
	if missing.From != preflight.FromGate {
		t.Errorf("From = %d, want FromGate", missing.From)
	}
	if !strings.Contains(err.Error(), "install Node.js") {
		t.Errorf("error = %q, want it to carry the npx hint", err.Error())
	}
}

func TestRunSkipsAlreadyInstalledGlobals(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()

	st := &rayconfig.State{}
	st.AddGlobal("headroom")
	st.AddGlobal("code_graph")
	if err := st.Save(home.StatePath); err != nil {
		t.Fatal(err)
	}

	fr := &runner.FakeRunner{}
	opts := Options{Profile: "test", Target: target, Out: &bytes.Buffer{}}
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
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()

	fr := &runner.FakeRunner{}
	opts := Options{Profile: "test", Target: target, NoGlobal: true, Out: &bytes.Buffer{}}
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

// Sem download não há cache de componente: a fonte é sempre
// home.ComponentsDir, lida e copiada direto a cada Run — copiar do disco
// local já é o caminho barato, não há nada a cachear. Este teste prova que
// dois targets diferentes recebem o mesmo conteúdo, cada um com sua própria
// cópia independente.
func TestRunCopiesComponentToEachTargetIndependently(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())
	target1 := t.TempDir()
	target2 := t.TempDir()

	opts1 := Options{Profile: "test", Target: target1, Out: &bytes.Buffer{}}
	if _, err := Run(&runner.FakeRunner{}, allFound, opts1, home); err != nil {
		t.Fatalf("Run() 1st run error = %v", err)
	}
	opts2 := Options{Profile: "test", Target: target2, Out: &bytes.Buffer{}}
	if _, err := Run(&runner.FakeRunner{}, allFound, opts2, home); err != nil {
		t.Fatalf("Run() 2nd run error = %v", err)
	}

	for _, target := range []string{target1, target2} {
		data, err := os.ReadFile(filepath.Join(target, ".claude/skills/s/SKILL.md"))
		if err != nil {
			t.Fatalf("content missing in %s: %v", target, err)
		}
		if string(data) != "# s" {
			t.Errorf("content in %s = %q, want %q", target, data, "# s")
		}
	}
}

func TestRunWritesProfileRecord(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()

	opts := Options{Profile: "test", Target: target, Out: &bytes.Buffer{}}
	sum, err := Run(&runner.FakeRunner{}, allFound, opts, home)
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

func TestRunWritesPristineHashForCopiedComponent(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()

	opts := Options{Profile: "test", Target: target, Out: &bytes.Buffer{}}
	sum, err := Run(&runner.FakeRunner{}, allFound, opts, home)
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
	pristine, ok := st.PristineHash(target, "s")
	if !ok {
		t.Fatal("PristineHash() ok = false, want a pristine hash recorded after Run")
	}
	if pristine != onDiskHash {
		t.Errorf("PristineHash() = %q, want it to match the on-disk hash %q", pristine, onDiskHash)
	}
}

// Um segundo componente, com Dest diferente (.claude/agents em vez de
// .claude/skills), prova que a cópia local não está amarrada a um único
// destino fixo.
func TestRunCopiesMultipleComponentsToTheirOwnDest(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "s")
	seedComponent(t, home, "reviewer")
	p := testProfile()
	p.Components = append(p.Components, profile.Component{Name: "reviewer", Dest: ".claude/agents"})
	writeProfile(t, home.ProfilesDir, p)
	target := t.TempDir()

	opts := Options{Profile: "test", Target: target, Out: &bytes.Buffer{}}
	sum, err := Run(&runner.FakeRunner{}, allFound, opts, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if sum.HadFailure {
		t.Fatalf("HadFailure = true, Failed = %v", sum.Failed)
	}
	if _, err := os.Stat(filepath.Join(target, ".claude/skills/s/SKILL.md")); err != nil {
		t.Fatalf("first component content missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, ".claude/agents/reviewer/SKILL.md")); err != nil {
		t.Fatalf("second component content missing at its own dest: %v", err)
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
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()

	sum, err := Run(&runner.FakeRunner{}, allFound, Options{
		Profile: "test", Target: target, Out: &bytes.Buffer{},
	}, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !hasWarning(sum, "brain integration is on but no brain is configured") {
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
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()

	ghost := filepath.Join(t.TempDir(), "nao-existe")
	t.Setenv("RAY_BRAIN", ghost)

	sum, err := Run(&runner.FakeRunner{}, allFound, Options{
		Profile: "test", Target: target, Out: &bytes.Buffer{},
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
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()

	brain := t.TempDir()
	t.Setenv("RAY_BRAIN", brain)

	sum, err := Run(&runner.FakeRunner{}, allFound, Options{
		Profile: "test", Target: target, Out: &bytes.Buffer{},
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
	seedComponent(t, home, "s")
	// testProfile() já liga Headroom/Brain/CodeGraph, então installer.Resolve
	// devolve servidores e mcp.WriteServers escreve o .mcp.json.
	writeProfile(t, home.ProfilesDir, testProfile())

	sum, err := Run(&runner.FakeRunner{}, allFound,
		Options{Profile: "test", Target: target}, home)
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

func TestVersionedPathsCollapsesToTopLevelEntries(t *testing.T) {
	got := versionedPaths(t.TempDir(), []string{
		"CLAUDE.md",
		".claude/hooks/session-start.sh",
		".claude/handoff.md",
		"docs/README.md",
		"docs/architecture.md",
		".gitignore",
		".mcp.json",
	})
	want := []string{".claude", ".gitignore", ".mcp.json", "CLAUDE.md", "docs"}
	if !slices.Equal(got, want) {
		t.Errorf("versionedPaths() = %v, want %v", got, want)
	}
}

func TestVersionedPathsIsDeterministic(t *testing.T) {
	target := t.TempDir()
	in := []string{"docs/a.md", "CLAUDE.md", ".claude/x", "docs/b.md"}
	first := versionedPaths(target, in)
	second := versionedPaths(target, in)
	if !slices.Equal(first, second) {
		t.Errorf("versionedPaths() is not deterministic: %v vs %v", first, second)
	}
}

// TestVersionedPathsNormalizesDotSlashPrefix trava a perda silenciosa que a
// revisão do Codex achou: "./x" tinha a primeira barra no índice 1, virava "."
// e caía no guarda de descarte. O arquivo sumia do `git add` — exatamente a
// falha que o rodapé existe para impedir.
func TestVersionedPathsNormalizesDotSlashPrefix(t *testing.T) {
	got := versionedPaths(t.TempDir(), []string{"./CLAUDE.md", "./docs/a.md"})
	want := []string{"CLAUDE.md", "docs"}
	if !slices.Equal(got, want) {
		t.Errorf("versionedPaths() = %v, want %v", got, want)
	}
}

// TestVersionedPathsRelativizesAbsoluteInsideTarget cobre o outro achado: com a
// barra no índice 0 nada era truncado e o caminho absoluto ia inteiro para o
// `git add`. Dentro do target ele tem tradução exata; usá-la é melhor que
// descartar, que devolveria a perda silenciosa por outra porta.
func TestVersionedPathsRelativizesAbsoluteInsideTarget(t *testing.T) {
	target := t.TempDir()
	got := versionedPaths(target, []string{
		filepath.Join(target, "docs", "a.md"),
		filepath.Join(target, ".mcp.json"),
	})
	want := []string{".mcp.json", "docs"}
	if !slices.Equal(got, want) {
		t.Errorf("versionedPaths() = %v, want %v", got, want)
	}
}

// TestVersionedPathsDropsPathsOutsideTarget: o rodapé só pode anunciar o que o
// `ray` escreveu dentro do target. Caminho de fora não é ambiente vendorizado, e
// mandar `git add` nele é pior que omitir.
func TestVersionedPathsDropsPathsOutsideTarget(t *testing.T) {
	target, outside := t.TempDir(), t.TempDir()
	got := versionedPaths(target, []string{
		filepath.Join(outside, "segredo.txt"),
		"../fora.md",
		"CLAUDE.md",
	})
	want := []string{"CLAUDE.md"}
	if !slices.Equal(got, want) {
		t.Errorf("versionedPaths() = %v, want %v", got, want)
	}
}

func TestInGitRepoDetectsAncestorDotGit(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "a", "b")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if !inGitRepo(nested) {
		t.Error("inGitRepo() = false for a directory under a repo root")
	}
	if inGitRepo(t.TempDir()) {
		t.Error("inGitRepo() = true outside any repo")
	}
}
