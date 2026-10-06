package initai

import (
	"bytes"
	"context"
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
		Integrations: profile.Integrations{Headroom: true, CodeGraph: true},
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

	missingPython := stubLooker{"npx": true, "node": true, "uv": true}
	opts := Options{Profile: "test", Target: target, Out: &bytes.Buffer{}}

	_, err := Run(&runner.FakeRunner{}, missingPython, opts, home)
	if err == nil {
		t.Fatal("Run() = nil error, want error when python3.10+ is missing")
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

	missingPython := stubLooker{"npx": true, "node": true, "uv": true}
	opts := Options{Profile: "test", Target: t.TempDir(), Out: &bytes.Buffer{}}

	_, err := Run(&runner.FakeRunner{}, missingPython, opts, home)

	var missing *preflight.MissingRequiredError
	if !errors.As(err, &missing) {
		t.Fatalf("Run() error = %v (%T), want a *preflight.MissingRequiredError", err, err)
	}
	if missing.From != preflight.FromGate {
		t.Errorf("From = %d, want FromGate", missing.From)
	}
	if !strings.Contains(err.Error(), "install Python 3.10+") {
		t.Errorf("error = %q, want it to carry the python hint", err.Error())
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

// Rodar `init ai` de novo não pode destruir a edição local de um componente:
// a decisão é a mesma do `ray update` (store.DecideOverwrite), e a linha-base
// só se move quando o conteúdo é regravado — senão a prova da edição some.
func TestRunTwicePreservesEditedComponentAndItsPristine(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	opts := Options{Profile: "test", Target: target, Out: &bytes.Buffer{}}

	if _, err := Run(&runner.FakeRunner{}, allFound, opts, home); err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	st := store.New(home.StoreDir)
	pristineBefore, ok := st.PristineHash(target, "s")
	if !ok {
		t.Fatal("PristineHash() ok = false after first Run")
	}

	skill := filepath.Join(target, ".claude", "skills", "s", "SKILL.md")
	const edited = "# edited by the user"
	if err := os.WriteFile(skill, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	// A fonte também muda: sem isso, regravar a linha-base com o hash do que
	// acabou de ser escrito daria o mesmo valor e a asserção abaixo seria vazia.
	if err := os.WriteFile(filepath.Join(home.ComponentsDir, "s", "SKILL.md"), []byte("# new upstream"), 0o644); err != nil {
		t.Fatal(err)
	}

	sum, err := Run(&runner.FakeRunner{}, allFound, opts, home)
	if err != nil {
		t.Fatalf("second Run() error = %v", err)
	}

	got, err := os.ReadFile(skill)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != edited {
		t.Errorf("SKILL.md = %q, want the user's edit %q preserved", got, edited)
	}
	pristineAfter, _ := st.PristineHash(target, "s")
	if pristineAfter != pristineBefore {
		t.Errorf("PristineHash() = %q after second Run, want it unchanged (%q)", pristineAfter, pristineBefore)
	}
	if !slices.ContainsFunc(sum.Warnings, func(w string) bool { return strings.Contains(w, "edited locally") }) {
		t.Errorf("Warnings = %v, want one explaining the component was edited locally", sum.Warnings)
	}
}

// O componente preservado precisa aparecer em Skipped: o passo do scaffold
// atribuía o próprio Skipped por cima e descartava o que o passo dos
// componentes já tinha registrado.
func TestRunKeepsEditedComponentInSkippedAfterScaffold(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	opts := Options{Profile: "test", Target: target, Out: &bytes.Buffer{}}

	if _, err := Run(&runner.FakeRunner{}, allFound, opts, home); err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	skill := filepath.Join(target, ".claude", "skills", "s", "SKILL.md")
	if err := os.WriteFile(skill, []byte("# edited by the user"), 0o644); err != nil {
		t.Fatal(err)
	}

	sum, err := Run(&runner.FakeRunner{}, allFound, opts, home)
	if err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	if !slices.Contains(sum.Skipped, "s") {
		t.Errorf("Skipped = %v, want it to list the preserved component %q", sum.Skipped, "s")
	}
	// O scaffold também pula o que já existe: os dois conjuntos se somam.
	if !slices.Contains(sum.Skipped, "CLAUDE.md") {
		t.Errorf("Skipped = %v, want the scaffold's own skip (CLAUDE.md) kept too", sum.Skipped)
	}
}

// Regressão da política: componente intacto acompanha a fonte. Sem isto a
// correção de preservar edição poderia virar "nunca atualiza".
func TestRunTwiceRefreshesUneditedComponentFromUpdatedSource(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	opts := Options{Profile: "test", Target: target, Out: &bytes.Buffer{}}

	if _, err := Run(&runner.FakeRunner{}, allFound, opts, home); err != nil {
		t.Fatalf("first Run() error = %v", err)
	}

	srcDir := filepath.Join(home.ComponentsDir, "s")
	const updated = "# updated upstream"
	if err := os.WriteFile(filepath.Join(srcDir, "SKILL.md"), []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "NEW.md"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Run(&runner.FakeRunner{}, allFound, opts, home); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}

	got, err := os.ReadFile(filepath.Join(target, ".claude", "skills", "s", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != updated {
		t.Errorf("SKILL.md = %q, want the updated source %q", got, updated)
	}
	if _, err := os.Stat(filepath.Join(target, ".claude", "skills", "s", "NEW.md")); err != nil {
		t.Errorf("NEW.md missing after refresh: %v", err)
	}
	fresh, err := store.HashTree(srcDir)
	if err != nil {
		t.Fatal(err)
	}
	if pristine, _ := store.New(home.StoreDir).PristineHash(target, "s"); pristine != fresh {
		t.Errorf("PristineHash() = %q, want the new source hash %q", pristine, fresh)
	}
}

// Um arquivo removido da fonte não pode sobrar no destino de um componente
// intacto: a cópia é limpa, e é isso que mantém disco == fonte.
func TestRunTwiceDropsFileRemovedFromSourceOfUneditedComponent(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "s")
	srcDir := filepath.Join(home.ComponentsDir, "s")
	if err := os.WriteFile(filepath.Join(srcDir, "OLD.md"), []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	opts := Options{Profile: "test", Target: target, Out: &bytes.Buffer{}}

	if _, err := Run(&runner.FakeRunner{}, allFound, opts, home); err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	if err := os.Remove(filepath.Join(srcDir, "OLD.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := Run(&runner.FakeRunner{}, allFound, opts, home); err != nil {
		t.Fatalf("second Run() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, ".claude", "skills", "s", "OLD.md")); !os.IsNotExist(err) {
		t.Errorf("OLD.md still on disk after being removed from the source (err = %v)", err)
	}
}

func TestRunForceOverwritesEditedComponentAndResetsPristine(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	opts := Options{Profile: "test", Target: target, Out: &bytes.Buffer{}}

	if _, err := Run(&runner.FakeRunner{}, allFound, opts, home); err != nil {
		t.Fatalf("first Run() error = %v", err)
	}
	skill := filepath.Join(target, ".claude", "skills", "s", "SKILL.md")
	if err := os.WriteFile(skill, []byte("# edited by the user"), 0o644); err != nil {
		t.Fatal(err)
	}

	opts.Force = true
	if _, err := Run(&runner.FakeRunner{}, allFound, opts, home); err != nil {
		t.Fatalf("forced Run() error = %v", err)
	}

	got, err := os.ReadFile(skill)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# s" {
		t.Errorf("SKILL.md = %q, want the source content %q after --force", got, "# s")
	}
	fresh, err := store.HashTree(filepath.Join(home.ComponentsDir, "s"))
	if err != nil {
		t.Fatal(err)
	}
	if pristine, _ := store.New(home.StoreDir).PristineHash(target, "s"); pristine != fresh {
		t.Errorf("PristineHash() = %q, want %q after --force", pristine, fresh)
	}
}

// Alvo clonado: o destino existe mas não há linha-base (ela mora no store, que
// não viaja com o repo). Igual à fonte → copia; diferente → ambíguo, preserva.
func TestRunWithoutPristineKeepsDivergentComponentAndCopiesIdenticalOne(t *testing.T) {
	cases := []struct {
		name     string
		onDisk   string
		wantDisk string
	}{
		{"divergent is preserved", "# edited by the user", "# edited by the user"},
		{"identical is refreshed", "# s", "# s"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := newHome(t)
			seedComponent(t, home, "s")
			writeProfile(t, home.ProfilesDir, testProfile())
			target := t.TempDir()
			dest := filepath.Join(target, ".claude", "skills", "s")
			if err := os.MkdirAll(dest, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dest, "SKILL.md"), []byte(tc.onDisk), 0o644); err != nil {
				t.Fatal(err)
			}

			opts := Options{Profile: "test", Target: target, Out: &bytes.Buffer{}}
			sum, err := Run(&runner.FakeRunner{}, allFound, opts, home)
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}

			got, err := os.ReadFile(filepath.Join(dest, "SKILL.md"))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.wantDisk {
				t.Errorf("SKILL.md = %q, want %q", got, tc.wantDisk)
			}
			_, hasPristine := store.New(home.StoreDir).PristineHash(target, "s")
			preserved := slices.Contains(sum.Skipped, "s")
			if tc.onDisk != "# s" {
				if !preserved || hasPristine {
					t.Errorf("preserved = %v, hasPristine = %v; want preserved and no baseline invented", preserved, hasPristine)
				}
			} else if preserved || !hasPristine {
				t.Errorf("preserved = %v, hasPristine = %v; want refreshed with a baseline recorded", preserved, hasPristine)
			}
		})
	}
}

// O dry-run tem de contar a mesma história que a execução real: componente
// editado aparece como preservado, não como cópia, e nada é gravado.
func TestRunDryRunReportsPreserveForEditedComponentAndCopyForUneditedOne(t *testing.T) {
	cases := []struct {
		name       string
		edit       bool
		wantOut    string
		notWantOut string
		wantSkip   bool
	}{
		{"edited is preserved", true, "+ preserve s (edited locally)", "+ copy s", true},
		{"unedited is copied", false, "+ copy s", "+ preserve s", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := newHome(t)
			seedComponent(t, home, "s")
			writeProfile(t, home.ProfilesDir, testProfile())
			target := t.TempDir()
			if _, err := Run(&runner.FakeRunner{}, allFound, Options{Profile: "test", Target: target, Out: &bytes.Buffer{}}, home); err != nil {
				t.Fatalf("first Run() error = %v", err)
			}
			skill := filepath.Join(target, ".claude", "skills", "s", "SKILL.md")
			wantDisk := "# s"
			if tc.edit {
				wantDisk = "# edited by the user"
				if err := os.WriteFile(skill, []byte(wantDisk), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			// A fonte muda, para que "copiar" tivesse efeito visível se executasse.
			if err := os.WriteFile(filepath.Join(home.ComponentsDir, "s", "SKILL.md"), []byte("# new upstream"), 0o644); err != nil {
				t.Fatal(err)
			}
			pristineBefore, _ := store.New(home.StoreDir).PristineHash(target, "s")

			var out bytes.Buffer
			sum, err := Run(&runner.FakeRunner{}, allFound, Options{Profile: "test", Target: target, DryRun: true, Out: &out}, home)
			if err != nil {
				t.Fatalf("dry-run error = %v", err)
			}

			if !strings.Contains(out.String(), tc.wantOut) {
				t.Errorf("output = %q, want it to contain %q", out.String(), tc.wantOut)
			}
			if strings.Contains(out.String(), tc.notWantOut) {
				t.Errorf("output = %q, must not contain %q", out.String(), tc.notWantOut)
			}
			if got := slices.Contains(sum.Skipped, "s"); got != tc.wantSkip {
				t.Errorf("Skipped = %v, want component skipped = %v", sum.Skipped, tc.wantSkip)
			}
			if got, _ := os.ReadFile(skill); string(got) != wantDisk {
				t.Errorf("SKILL.md = %q, dry-run must not write (want %q)", got, wantDisk)
			}
			if after, _ := store.New(home.StoreDir).PristineHash(target, "s"); after != pristineBefore {
				t.Errorf("PristineHash() changed under dry-run: %q -> %q", pristineBefore, after)
			}
		})
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

func TestRunRecordsMCPJSONInCreated(t *testing.T) {
	home, target := newHome(t), t.TempDir()
	seedComponent(t, home, "s")
	// testProfile() já liga Headroom/CodeGraph, então installer.Resolve
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

// O init ai carrega a receita pelo mesmo Load que valida: uma receita hostil no
// disco falha antes de qualquer efeito no alvo.
func TestRunRefusesHostileRecipeBeforeAnyEffectOnTarget(t *testing.T) {
	recipes := map[string]string{
		"component name climbs": "name: hostile\ncomponents:\n  - name: ..\n    dest: .claude/skills\n",
		"component dest climbs": "name: hostile\ncomponents:\n  - name: s\n    dest: ../../elsewhere\n",
		"scaffold path climbs":  "name: hostile\nscaffold:\n  files:\n    - path: ../../outside.md\n",
		"scaffold template":     "name: hostile\nscaffold:\n  files:\n    - path: CLAUDE.md\n      template: ../../secret\n",
	}
	for label, yamlText := range recipes {
		t.Run(label, func(t *testing.T) {
			home := newHome(t)
			seedComponent(t, home, "s")
			if err := os.MkdirAll(home.ProfilesDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(home.ProfilesDir, "hostile.yaml"), []byte(yamlText), 0o644); err != nil {
				t.Fatal(err)
			}
			target := t.TempDir()

			_, err := Run(&runner.FakeRunner{}, allFound, Options{Profile: "hostile", Target: target, Force: true, Out: &bytes.Buffer{}}, home)
			if err == nil || !strings.Contains(err.Error(), "invalid profile") {
				t.Fatalf("Run() error = %v, want an invalid profile error", err)
			}
			entries, rerr := os.ReadDir(target)
			if rerr != nil {
				t.Fatal(rerr)
			}
			if len(entries) != 0 {
				t.Errorf("target has %d entries after a refused recipe, want none: %v", len(entries), entries)
			}
		})
	}
}

// Mesmo contrato do update: pasta que o hash não consegue ler é preservada.
func TestRunTwicePreservesEditedComponentWhoseHashCannotBeComputed(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(map[bool]string{false: "without force", true: "with force"}[force], func(t *testing.T) {
			home := newHome(t)
			seedComponent(t, home, "s")
			writeProfile(t, home.ProfilesDir, testProfile())
			target := t.TempDir()
			opts := Options{Profile: "test", Target: target, Out: &bytes.Buffer{}}
			if _, err := Run(&runner.FakeRunner{}, allFound, opts, home); err != nil {
				t.Fatalf("first Run() error = %v", err)
			}
			skillDir := filepath.Join(target, ".claude", "skills", "s")
			if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# edited by the user"), 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(target, "no-such-target"), filepath.Join(skillDir, "dangling")); err != nil {
				t.Skipf("symlinks unavailable: %v", err)
			}

			opts.Force = force
			sum, err := Run(&runner.FakeRunner{}, allFound, opts, home)
			if err != nil {
				t.Fatalf("second Run() error = %v", err)
			}

			got, _ := os.ReadFile(filepath.Join(skillDir, "SKILL.md"))
			if force {
				if string(got) != "# s" {
					t.Errorf("SKILL.md = %q, want the source content after --force", got)
				}
				return
			}
			if string(got) != "# edited by the user" {
				t.Errorf("SKILL.md = %q, want the user's edit preserved", got)
			}
			if !slices.Contains(sum.Skipped, "s") {
				t.Errorf("Skipped = %v, want it to include the component", sum.Skipped)
			}
		})
	}
}

func settingsEventCommands(t *testing.T, target, event string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(target, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatal(err)
	}
	var cmds []string
	for _, e := range doc.Hooks[event] {
		for _, h := range e.Hooks {
			cmds = append(cmds, h.Command)
		}
	}
	return cmds
}

// Os hooks que a receita declara e os do ray se somam por evento: antes, a
// união rasa deixava os do ray e apagava os da receita antes de chegar ao arquivo.
func TestRunKeepsRecipeHooksAlongsideTheRaysAndDoesNotDuplicateThem(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "s")
	p := testProfile()
	p.Scaffold.Settings = map[string]any{
		"model": "opus",
		"hooks": map[string]any{
			"SessionStart": []any{map[string]any{"hooks": []any{map[string]any{"type": "command", "command": "bash my-recipe-hook.sh"}}}},
		},
	}
	writeProfile(t, home.ProfilesDir, p)
	target := t.TempDir()
	opts := Options{Profile: "test", Target: target, Out: &bytes.Buffer{}}

	for run := 1; run <= 2; run++ {
		if _, err := Run(&runner.FakeRunner{}, allFound, opts, home); err != nil {
			t.Fatalf("Run() #%d error = %v", run, err)
		}
		got := settingsEventCommands(t, target, "SessionStart")
		var recipe, ray int
		for _, c := range got {
			switch c {
			case "bash my-recipe-hook.sh":
				recipe++
			case "bash .claude/hooks/session-start.sh":
				ray++
			}
		}
		if recipe != 1 || ray != 1 {
			t.Errorf("run %d: SessionStart = %v, want the recipe's hook and the ray's exactly once each", run, got)
		}
	}
}

// O usuário que já tem hooks e model no settings.json não os perde ao rodar init ai.
func TestRunKeepsTheUsersExistingSettings(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	if err := os.MkdirAll(filepath.Join(target, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	mine := `{"model": "sonnet", "hooks": {"Stop": [{"hooks": [{"type": "command", "command": "notify-send done"}]}]}}`
	if err := os.WriteFile(filepath.Join(target, ".claude", "settings.json"), []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Run(&runner.FakeRunner{}, allFound, Options{Profile: "test", Target: target, Out: &bytes.Buffer{}}, home); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if got := settingsEventCommands(t, target, "Stop"); !slices.Equal(got, []string{"notify-send done"}) {
		t.Errorf("Stop = %v, want the user's hook kept", got)
	}
	data, _ := os.ReadFile(filepath.Join(target, ".claude", "settings.json"))
	if !strings.Contains(string(data), `"model": "sonnet"`) {
		t.Errorf("settings.json = %s, want the user's model kept", data)
	}
}

func homeIsUntouched(t *testing.T, home Home) {
	t.Helper()
	for name, path := range map[string]string{
		"ProfilesDir": home.ProfilesDir, "TemplatesDir": home.TemplatesDir,
		"StoreDir": home.StoreDir, "StatePath": home.StatePath, "ConfigPath": home.ConfigPath,
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("%s (%s) exists after a dry-run (stat err = %v)", name, path, err)
		}
	}
}

// Um dry-run numa máquina virgem usa os perfis de fábrica em memória e não
// deixa nada em ~/.ray nem no projeto — nem com --force.
func TestRunDryRunLeavesTheRayHomeUntouchedOnAVirginMachine(t *testing.T) {
	for _, force := range []bool{false, true} {
		home := newHome(t)
		target := t.TempDir()
		var out bytes.Buffer
		_, err := Run(&runner.FakeRunner{}, allFound, Options{Profile: "base", Target: target, DryRun: true, Force: force, Out: &out}, home)
		if err != nil {
			t.Fatalf("dry-run (force=%v) error = %v", force, err)
		}
		homeIsUntouched(t, home)
		if entries, _ := os.ReadDir(target); len(entries) != 0 {
			t.Errorf("force=%v: target has %d entries after a dry-run, want none", force, len(entries))
		}
		if !strings.Contains(out.String(), "+ ") {
			t.Errorf("force=%v: dry-run printed no plan: %q", force, out.String())
		}
	}
}

// O overlay de templates é editável pelo usuário; um dry-run --force não pode
// apagá-lo.
func TestRunDryRunForceKeepsAnEditedTemplate(t *testing.T) {
	home := newHome(t)
	tmpl := filepath.Join(home.TemplatesDir, "claude", "handoff.md.tmpl")
	if err := os.MkdirAll(filepath.Dir(tmpl), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(tmpl, []byte("# my custom template"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := Run(&runner.FakeRunner{}, allFound, Options{Profile: "base", Target: t.TempDir(), DryRun: true, Force: true, Out: &bytes.Buffer{}}, home); err != nil {
		t.Fatalf("dry-run error = %v", err)
	}
	if got, _ := os.ReadFile(tmpl); string(got) != "# my custom template" {
		t.Errorf("template = %q, want the edit kept", got)
	}
}

// O `graphify update .` indexa o que o projeto já tem; numa pasta sem código ele
// sai com 1 ("nothing to rebuild"), e isso não é o ambiente falhando — o índice
// é regenerável. Vira aviso: o resumo diz o que aconteceu e o comando não
// acusa erro.
func TestRunPerProjectCommandFailureIsAWarningNotAFailure(t *testing.T) {
	cases := map[string]*runner.FakeRunner{
		"exit code 1": {Results: map[string]runner.Result{"graphify update .": {ExitCode: 1}}},
	}
	for name, fr := range cases {
		t.Run(name, func(t *testing.T) {
			home := newHome(t)
			seedComponent(t, home, "s")
			writeProfile(t, home.ProfilesDir, testProfile())
			opts := Options{Profile: "test", Target: t.TempDir(), NoGlobal: true, Out: &bytes.Buffer{}}

			sum, err := Run(fr, allFound, opts, home)
			if err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if sum.HadFailure || len(sum.Failed) != 0 {
				t.Errorf("HadFailure = %v, Failed = %v; want none: an index that could not be built is not a failed environment", sum.HadFailure, sum.Failed)
			}
			if !slices.ContainsFunc(sum.Warnings, func(w string) bool {
				return strings.Contains(w, "graphify update .") && strings.Contains(w, "failed")
			}) {
				t.Errorf("Warnings = %v, want one saying `graphify update .` failed", sum.Warnings)
			}
			if slices.Contains(sum.Installed, "graphify update .") {
				t.Errorf("Installed = %v, must not list the command that failed", sum.Installed)
			}
		})
	}
}

// Quando o comando nem chega a rodar (binário ausente, erro de exec), vale o
// mesmo: aviso, não falha.
func TestRunPerProjectCommandExecErrorIsAWarning(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())
	// NoGlobal: o erro de exec não pode derrubar também os passos globais.
	fr := &failOnlyRunner{match: "graphify update .", err: errors.New("exec: graphify not found")}

	sum, err := Run(fr, allFound, Options{Profile: "test", Target: t.TempDir(), NoGlobal: true, Out: &bytes.Buffer{}}, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if sum.HadFailure {
		t.Errorf("HadFailure = true, Failed = %v; want a warning only", sum.Failed)
	}
	if !slices.ContainsFunc(sum.Warnings, func(w string) bool { return strings.Contains(w, "graphify update .") }) {
		t.Errorf("Warnings = %v, want one mentioning the command", sum.Warnings)
	}
}

// failOnlyRunner devolve err só para o comando que casa com match.
type failOnlyRunner struct {
	match string
	err   error
}

func (f *failOnlyRunner) Run(_ context.Context, c runner.Command) (runner.Result, error) {
	if c.String() == f.match {
		return runner.Result{}, f.err
	}
	return runner.Result{ExitCode: 0}, nil
}

// Controle: o que continua sendo falha. Uma instalação global que não roda
// compromete a máquina, e aí o resumo e o exit têm de acusar.
func TestRunGlobalInstallFailureStillFailsTheRun(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())

	sum, err := Run(&runner.FakeRunner{Err: errors.New("boom")}, allFound, Options{Profile: "test", Target: t.TempDir(), Out: &bytes.Buffer{}}, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !sum.HadFailure {
		t.Error("HadFailure = false, want true when the global installs fail")
	}
}

// O resumo diz O QUE falhou e, agora, POR QUÊ: exit code e a última linha do
// stderr, em vez de só o nome do comando.
func TestRunFailureCarriesItsReason(t *testing.T) {
	t.Run("global install", func(t *testing.T) {
		home := newHome(t)
		seedComponent(t, home, "s")
		writeProfile(t, home.ProfilesDir, testProfile())
		fr := &runner.FakeRunner{Results: map[string]runner.Result{
			"uv tool install graphifyy": {ExitCode: 2, Stderr: "error: No space left on device\n"},
		}}

		sum, err := Run(fr, allFound, Options{Profile: "test", Target: t.TempDir(), Out: &bytes.Buffer{}}, home)
		if err != nil {
			t.Fatal(err)
		}
		if !sum.HadFailure {
			t.Fatal("HadFailure = false, want the failed install to fail the run")
		}
		if !slices.ContainsFunc(sum.Warnings, func(w string) bool {
			return strings.Contains(w, "uv tool install graphifyy") && strings.Contains(w, "exit 2: error: No space left on device")
		}) {
			t.Errorf("Warnings = %v, want the failed command with its exit code and stderr", sum.Warnings)
		}
	})
	t.Run("per-project command", func(t *testing.T) {
		home := newHome(t)
		seedComponent(t, home, "s")
		writeProfile(t, home.ProfilesDir, testProfile())
		fr := &runner.FakeRunner{Results: map[string]runner.Result{
			"graphify update .": {ExitCode: 1, Stderr: "[graphify watch] No code files found - nothing to rebuild.\n"},
		}}

		sum, err := Run(fr, allFound, Options{Profile: "test", Target: t.TempDir(), NoGlobal: true, Out: &bytes.Buffer{}}, home)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.ContainsFunc(sum.Warnings, func(w string) bool { return strings.Contains(w, "No code files found") }) {
			t.Errorf("Warnings = %v, want the graphify reason in the warning", sum.Warnings)
		}
	})
}
