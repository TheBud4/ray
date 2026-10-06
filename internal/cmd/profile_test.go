package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/TheBud4/ray/internal/initai"
	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/store"
)

func TestRunProfileListIncludesDefaultsAndExtra(t *testing.T) {
	dir := t.TempDir()
	if err := runProfileAdd(dir, "custom"); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runProfileList(dir, t.TempDir(), &out); err != nil {
		t.Fatalf("runProfileList() error = %v", err)
	}

	got := out.String()
	for _, name := range []string{"go", "web", "flutter", "custom"} {
		if !strings.Contains(got, name) {
			t.Errorf("output = %q, want it to contain %q", got, name)
		}
	}
}

// A lista é o único lugar onde uma receita quebrada pode ser descoberta: quem
// não sabe o nome não tem o que passar para `profile show`. Marca e motivo
// curto na própria linha; o erro completo continua sendo do show.
func TestRunProfileListMarksBrokenProfiles(t *testing.T) {
	dir := t.TempDir()
	if err := profile.EnsureDir(dir, store.New(t.TempDir())); err != nil {
		t.Fatal(err)
	}
	const bad = "name: badsemantic\ndescription: parses fine\ncomponents:\n  - name: ctx7\n    type: mcp\n    via: aitmpl\n    ref: context7\n"
	if err := os.WriteFile(filepath.Join(dir, "badsemantic.yaml"), []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "broken.yaml"), []byte(":\n  - ["), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runProfileList(dir, t.TempDir(), &out); err != nil {
		t.Fatalf("runProfileList() error = %v", err)
	}

	got := out.String()
	for _, want := range []string{"badsemantic", "invalid:", "broken.yaml", "unreadable:"} {
		if !strings.Contains(got, want) {
			t.Errorf("output = %q, want it to contain %q", got, want)
		}
	}
	// A receita sã não pode ganhar ruído por causa das vizinhas.
	for _, line := range strings.Split(strings.TrimSpace(got), "\n") {
		if strings.HasPrefix(line, "go —") && strings.Contains(line, "(") {
			t.Errorf("healthy profile line = %q, want no marker", line)
		}
	}
}

func TestRunProfileShowPrintsComponents(t *testing.T) {
	dir := t.TempDir()
	p := &profile.Profile{
		Name:        "test",
		Description: "a test profile",
		Components:  []profile.Component{{Name: "s", Dest: ".claude/skills"}},
	}
	if err := profile.WriteNew(dir, p); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runProfileShow(dir, "test", &out); err != nil {
		t.Fatalf("runProfileShow() error = %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "s -> .claude/skills/s") {
		t.Errorf("output = %q, want the component's local name and dest, never a download command", got)
	}
}

// O graphify é o servidor sem args da receita, e era ele que saía como
// "graphify: graphify-mcp " — um espaço pendurado no fim da linha, porque o
// formato juntava os args mesmo quando não havia nenhum.
func TestRunProfileShowLeavesNoTrailingSpaceOnAServerWithoutArgs(t *testing.T) {
	dir := t.TempDir()
	p := &profile.Profile{
		Name:         "test",
		Description:  "a test profile",
		Integrations: profile.Integrations{CodeGraph: true},
	}
	if err := profile.WriteNew(dir, p); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runProfileShow(dir, "test", &out); err != nil {
		t.Fatalf("runProfileShow() error = %v", err)
	}

	for _, line := range strings.Split(out.String(), "\n") {
		if strings.TrimRight(line, " ") != line {
			t.Errorf("line %q has trailing whitespace", line)
		}
	}
}

func TestRunProfileAddCreatesAndRejectsDuplicate(t *testing.T) {
	dir := t.TempDir()

	if err := runProfileAdd(dir, "custom"); err != nil {
		t.Fatalf("runProfileAdd() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom.yaml")); err != nil {
		t.Fatalf("stat custom.yaml: %v", err)
	}

	if err := runProfileAdd(dir, "custom"); err == nil {
		t.Fatal("runProfileAdd() = nil error, want error on duplicate name")
	}
}

func TestRunProfileEditRequiresEditorEnv(t *testing.T) {
	t.Setenv("EDITOR", "")

	err := runProfileEdit(t.TempDir(), "go", func(editor, path string) error {
		t.Fatal("spawn should not be called when $EDITOR is unset")
		return nil
	})
	if err == nil {
		t.Fatal("runProfileEdit() = nil error, want error when $EDITOR is unset")
	}
}

func TestRunProfileEditSpawnsWithEditorAndPath(t *testing.T) {
	t.Setenv("EDITOR", "nano")
	dir := t.TempDir()

	var gotEditor, gotPath string
	err := runProfileEdit(dir, "go", func(editor, path string) error {
		gotEditor, gotPath = editor, path
		return nil
	})
	if err != nil {
		t.Fatalf("runProfileEdit() error = %v", err)
	}
	if gotEditor != "nano" {
		t.Errorf("editor = %q, want nano", gotEditor)
	}
	if gotPath != filepath.Join(dir, "go.yaml") {
		t.Errorf("path = %q, want %q", gotPath, filepath.Join(dir, "go.yaml"))
	}
}

func TestRunProfileRemoveDeletesAndErrorsOnMissing(t *testing.T) {
	dir := t.TempDir()
	if err := runProfileAdd(dir, "custom"); err != nil {
		t.Fatal(err)
	}

	if err := runProfileRemove(dir, "custom"); err != nil {
		t.Fatalf("runProfileRemove() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom.yaml")); !os.IsNotExist(err) {
		t.Fatalf("custom.yaml should be gone, stat err = %v", err)
	}

	if err := runProfileRemove(dir, "does-not-exist"); err == nil {
		t.Fatal("runProfileRemove() = nil error, want error removing a missing profile")
	}
}

// `profile edit` abre o editor sobre <dir>/<name>.yaml: um nome fora de dir não
// pode chegar ao spawn, onde o editor abriria (e salvaria) um arquivo alheio.
func TestRunProfileEditRefusesNameOutsideProfilesDir(t *testing.T) {
	t.Setenv("EDITOR", "ed")
	called := false
	err := runProfileEdit(t.TempDir(), "../config", func(editor, path string) error {
		called = true
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "single path element") {
		t.Errorf("runProfileEdit() = %v, want an error mentioning a single path element", err)
	}
	if called {
		t.Error("the editor was spawned for a name outside the profiles dir")
	}
}

// seedOutdatedFactoryProfile grava em dir um perfil de fábrica `name` antigo,
// nunca editado (com a linha-base igual ao que está em disco), como o ray teria
// deixado numa versão anterior. Devolve o conteúdo atual de fábrica.
func seedOutdatedFactoryProfile(t *testing.T, dir, storeDir, name string) (current []byte) {
	t.Helper()
	old := []byte("name: " + name + "\ndescription: an older factory profile\n")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".yaml"), old, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := store.New(storeDir).SetPristine(dir, name+".yaml", store.HashBytes(old)); err != nil {
		t.Fatal(err)
	}
	for _, p := range profile.Defaults() {
		if p.Name == name {
			data, err := yaml.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			return data
		}
	}
	t.Fatalf("no factory profile %q", name)
	return nil
}

func assertProfileIsCurrent(t *testing.T, dir, name string, current []byte) {
	t.Helper()
	got, err := os.ReadFile(filepath.Join(dir, name+".yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(current) {
		t.Errorf("%s.yaml = %q, want the current factory profile: an older, never-edited copy must not shadow the binary", name, got)
	}
}

// Os três comandos que garantem o diretório de receitas sincronizam os perfis de
// fábrica: um binário novo tem de chegar a quem nunca editou o arquivo.
func TestProfileListSyncsOutdatedFactoryProfiles(t *testing.T) {
	dir, storeDir := t.TempDir(), t.TempDir()
	current := seedOutdatedFactoryProfile(t, dir, storeDir, "go")

	if err := runProfileList(dir, storeDir, &bytes.Buffer{}); err != nil {
		t.Fatalf("runProfileList() error = %v", err)
	}
	assertProfileIsCurrent(t, dir, "go", current)
}

func TestNewSyncsOutdatedFactoryProfiles(t *testing.T) {
	t.Chdir(t.TempDir())
	home := newTestHome(t)
	current := seedOutdatedFactoryProfile(t, home.ProfilesDir, home.StoreDir, "base")

	if _, err := runNew(&runner.FakeRunner{}, allFound, home.ProfilesDir, "base", "myproj", true, false, initai.Options{}, home); err != nil {
		t.Fatalf("runNew() error = %v", err)
	}
	assertProfileIsCurrent(t, home.ProfilesDir, "base", current)
}

func TestInitAISyncsOutdatedFactoryProfiles(t *testing.T) {
	home := newTestHome(t)
	current := seedOutdatedFactoryProfile(t, home.ProfilesDir, home.StoreDir, "base")

	opts := initai.Options{Profile: "base", Target: t.TempDir(), Out: &bytes.Buffer{}}
	if _, err := initai.Run(&runner.FakeRunner{}, allFound, opts, home); err != nil {
		t.Fatalf("initai.Run() error = %v", err)
	}
	assertProfileIsCurrent(t, home.ProfilesDir, "base", current)
}
