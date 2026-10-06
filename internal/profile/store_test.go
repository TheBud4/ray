package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/TheBud4/ray/internal/store"
)

func TestEnsureDirCreatesDefaults(t *testing.T) {
	dir := t.TempDir()

	if err := EnsureDir(dir, store.New(t.TempDir())); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"go", "web", "flutter"} {
		path := filepath.Join(dir, name+".yaml")
		p, err := Load(path)
		if err != nil {
			t.Fatalf("Load(%s) = %v, want nil", path, err)
		}
		if p.Name != name {
			t.Errorf("Load(%s).Name = %q, want %q", path, p.Name, name)
		}
	}
}

func TestEnsureDirIdempotentNoOverwrite(t *testing.T) {
	dir := t.TempDir()

	if err := EnsureDir(dir, store.New(t.TempDir())); err != nil {
		t.Fatal(err)
	}

	goPath := filepath.Join(dir, "go.yaml")
	edited := "name: go\ndescription: EDITED\n"
	if err := os.WriteFile(goPath, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureDir(dir, store.New(t.TempDir())); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(goPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "EDITED") {
		t.Errorf("go.yaml was overwritten; got: %s", data)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	// RF-07 acrescentou o perfil `base` (init ai sem --profile) aos defaults.
	if len(entries) != 4 {
		t.Errorf("dir has %d entries, want 4 (go/web/flutter/base)", len(entries))
	}
}

func TestWriteNewAndRemove(t *testing.T) {
	dir := t.TempDir()

	if err := WriteNew(dir, Starter("mine")); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(filepath.Join(dir, "mine.yaml")); err != nil {
		t.Fatalf("Load(mine.yaml) = %v, want nil", err)
	}

	if err := WriteNew(dir, Starter("mine")); err == nil {
		t.Fatal("WriteNew() second call = nil, want error (already exists)")
	} else if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("WriteNew() error = %q, want it to contain \"already exists\"", err.Error())
	}

	if err := WriteNew(dir, Starter("")); err == nil {
		t.Fatal("WriteNew() with invalid profile = nil, want error")
	}

	if err := Remove(dir, "mine"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "mine.yaml")); !os.IsNotExist(err) {
		t.Errorf("mine.yaml still exists after Remove")
	}

	if err := Remove(dir, "mine"); err == nil {
		t.Fatal("Remove() second call = nil, want error")
	}
}

// Remove e WriteNew montam <dir>/<name>.yaml; um nome que escapa de dir não pode
// apagar nem criar nada fora dele. A sentinela vive no diretório pai, onde fica,
// por exemplo, o config.yaml do próprio ray.
func TestRemoveAndWriteNewRefuseNamesOutsideTheirDir(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "profiles")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(sentinel, []byte("keep: me\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"../config", "a/../../config", "..", filepath.Join(root, "config")} {
		if err := Remove(dir, name); err == nil || !strings.Contains(err.Error(), "single path element") {
			t.Errorf("Remove(%q) = %v, want an error mentioning a single path element", name, err)
		}
		if err := WriteNew(dir, Starter(name)); err == nil {
			t.Errorf("WriteNew(Starter(%q)) = nil, want an error", name)
		}
	}
	if got, err := os.ReadFile(sentinel); err != nil || string(got) != "keep: me\n" {
		t.Errorf("sentinel outside the profiles dir = %q, %v; want it untouched", got, err)
	}
	if _, err := os.Stat(filepath.Join(root, "x.yaml")); !os.IsNotExist(err) {
		t.Errorf("a file was created outside the profiles dir (stat err = %v)", err)
	}
}

func TestList(t *testing.T) {
	t.Run("nonexistent dir", func(t *testing.T) {
		entries, err := List(filepath.Join(t.TempDir(), "missing"))
		if err != nil {
			t.Fatal(err)
		}
		if entries != nil {
			t.Errorf("List() = %v, want nil", entries)
		}
	})

	t.Run("after EnsureDir", func(t *testing.T) {
		dir := t.TempDir()
		if err := EnsureDir(dir, store.New(t.TempDir())); err != nil {
			t.Fatal(err)
		}

		entries, err := List(dir)
		if err != nil {
			t.Fatal(err)
		}
		// RF-07 acrescentou o perfil `base` aos defaults.
		if len(entries) != 4 {
			t.Fatalf("List() returned %d entries, want 4", len(entries))
		}
		names := []string{entries[0].Name, entries[1].Name, entries[2].Name, entries[3].Name}
		want := []string{"base", "flutter", "go", "web"}
		for i, n := range names {
			if n != want[i] {
				t.Errorf("entries[%d].Name = %q, want %q (order: %v)", i, n, want[i], names)
			}
		}
		for _, e := range entries {
			if e.Description == "" {
				t.Errorf("entry %q has empty Description", e.Name)
			}
		}
	})

	// Este subteste exigia o contrário — que broken.yaml sumisse da lista.
	// Sumir é o defeito: quem não vê o nome não sabe que há arquivo para
	// inspecionar com `profile show`, que é onde o erro completo mora.
	t.Run("reveals broken yaml", func(t *testing.T) {
		dir := t.TempDir()
		if err := EnsureDir(dir, store.New(t.TempDir())); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "broken.yaml"), []byte(":\n  - ["), 0o644); err != nil {
			t.Fatal(err)
		}

		entries, err := List(dir)
		if err != nil {
			t.Fatal(err)
		}
		// RF-07 acrescentou o perfil `base` aos defaults.
		if len(entries) != 5 {
			t.Fatalf("List() returned %d entries, want 5 (broken.yaml included)", len(entries))
		}

		var broken *Entry
		for i := range entries {
			if entries[i].Name == "broken.yaml" {
				broken = &entries[i]
			}
		}
		if broken == nil {
			t.Fatalf("no entry named broken.yaml; got %v", entries)
		}
		// O nome é o do arquivo porque o conteúdo não parseia: não há campo
		// `name` de onde tirar outro.
		if broken.Problem == "" {
			t.Error("broken.yaml has an empty Problem; want the parse error")
		}
	})

	t.Run("flags a profile that parses but does not validate", func(t *testing.T) {
		dir := t.TempDir()
		// Componente sem `dest` parseia como YAML válido e é recusado pelo
		// Validate — todo componente precisa dizer onde é copiado.
		const bad = "name: badsemantic\ndescription: parses fine\ncomponents:\n  - name: ctx7\n"
		if err := os.WriteFile(filepath.Join(dir, "badsemantic.yaml"), []byte(bad), 0o644); err != nil {
			t.Fatal(err)
		}

		entries, err := List(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("List() returned %d entries, want 1", len(entries))
		}
		e := entries[0]
		if e.Name != "badsemantic" {
			t.Errorf("Name = %q, want %q", e.Name, "badsemantic")
		}
		if e.Problem == "" {
			t.Fatal("Problem is empty; an invalid profile must not list as healthy")
		}
		if !strings.Contains(e.Problem, "dest is required") {
			t.Errorf("Problem = %q, want it to name the offending reason", e.Problem)
		}
	})

	// O yaml.v3 devolve erro multi-linha quando o tipo não bate ("yaml:
	// unmarshal errors:\n  line N: ..."), e a lista é uma linha por receita.
	t.Run("problem is always a single line", func(t *testing.T) {
		dir := t.TempDir()
		const mismatch = "name: mismatch\ncomponents: not-a-list\n"
		if err := os.WriteFile(filepath.Join(dir, "mismatch.yaml"), []byte(mismatch), 0o644); err != nil {
			t.Fatal(err)
		}

		entries, err := List(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("List() returned %d entries, want 1", len(entries))
		}
		if p := entries[0].Problem; strings.Contains(p, "\n") {
			t.Errorf("Problem = %q, want it collapsed to a single line", p)
		}
	})

	t.Run("healthy profile has no problem", func(t *testing.T) {
		dir := t.TempDir()
		if err := EnsureDir(dir, store.New(t.TempDir())); err != nil {
			t.Fatal(err)
		}

		entries, err := List(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if e.Problem != "" {
				t.Errorf("seeded profile %q reports Problem = %q, want none", e.Name, e.Problem)
			}
		}
	})
}

// ---- EnsureDir sincroniza os perfis de fábrica -----------------------------

// factoryYAML é o que o ray grava para o perfil de fábrica name.
func factoryYAML(t *testing.T, name string) []byte {
	t.Helper()
	for _, p := range Defaults() {
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

// Cada linha da tabela da especificação: o que o EnsureDir faz com um perfil de
// fábrica conforme o arquivo em disco e a linha-base. A linha-base mora no store
// da máquina, com a mesma chave do overlay de templates (<dir>, <nome>.yaml).
func TestEnsureDirSyncsFactoryProfiles(t *testing.T) {
	old := []byte("name: go\ndescription: an older factory go\n")
	edited := []byte("name: go\ndescription: my own go\n")

	cases := []struct {
		name     string
		disk     []byte // nil = ausente
		pristine []byte // nil = sem linha-base
		want     []byte // conteúdo esperado em disco; nil = o de fábrica
	}{
		{"absent is written", nil, nil, nil},
		{"identical stays", factoryYAML(t, "go"), nil, nil},
		{"never edited and outdated is updated", old, old, nil},
		{"edited is kept", edited, old, edited},
		{"differs without a baseline is kept", old, nil, old},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			st := store.New(t.TempDir())
			path := filepath.Join(dir, "go.yaml")
			if tc.disk != nil {
				if err := os.WriteFile(path, tc.disk, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if tc.pristine != nil {
				if err := st.SetPristine(dir, "go.yaml", store.HashBytes(tc.pristine)); err != nil {
					t.Fatal(err)
				}
			}

			if err := EnsureDir(dir, st); err != nil {
				t.Fatalf("EnsureDir() error = %v", err)
			}

			want := tc.want
			if want == nil {
				want = factoryYAML(t, "go")
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != string(want) {
				t.Errorf("go.yaml = %q, want %q", got, want)
			}
		})
	}
}

// A linha-base acompanha o que o ray escreveu: depois de gravar ou atualizar,
// ela é a de fábrica; um perfil editado não a move.
func TestEnsureDirRecordsTheBaselineOnlyForWhatItWrote(t *testing.T) {
	factory := store.HashBytes(factoryYAML(t, "go"))
	old := []byte("name: go\ndescription: older\n")
	edited := []byte("name: go\ndescription: mine\n")

	t.Run("written", func(t *testing.T) {
		dir, st := t.TempDir(), store.New(t.TempDir())
		if err := EnsureDir(dir, st); err != nil {
			t.Fatal(err)
		}
		if got, ok := st.PristineHash(dir, "go.yaml"); !ok || got != factory {
			t.Errorf("baseline = (%q, %v), want the factory hash", got, ok)
		}
	})
	t.Run("updated", func(t *testing.T) {
		dir, st := t.TempDir(), store.New(t.TempDir())
		if err := os.WriteFile(filepath.Join(dir, "go.yaml"), old, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := st.SetPristine(dir, "go.yaml", store.HashBytes(old)); err != nil {
			t.Fatal(err)
		}
		if err := EnsureDir(dir, st); err != nil {
			t.Fatal(err)
		}
		if got, _ := st.PristineHash(dir, "go.yaml"); got != factory {
			t.Errorf("baseline = %q, want it moved to the factory hash", got)
		}
	})
	t.Run("edited keeps its baseline", func(t *testing.T) {
		dir, st := t.TempDir(), store.New(t.TempDir())
		if err := os.WriteFile(filepath.Join(dir, "go.yaml"), edited, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := st.SetPristine(dir, "go.yaml", store.HashBytes(old)); err != nil {
			t.Fatal(err)
		}
		if err := EnsureDir(dir, st); err != nil {
			t.Fatal(err)
		}
		if got, _ := st.PristineHash(dir, "go.yaml"); got != store.HashBytes(old) {
			t.Errorf("baseline = %q, want it untouched for an edited profile", got)
		}
	})
}

// Só perfil de fábrica é sincronizado; o resto da pasta é do usuário. E rodar de
// novo não regrava nada — nem o perfil, nem o pristine.yaml compartilhado.
func TestEnsureDirLeavesOtherProfilesAloneAndIsIdempotent(t *testing.T) {
	dir, storeRoot := t.TempDir(), t.TempDir()
	st := store.New(storeRoot)
	mine := []byte("name: mine\ndescription: not a factory profile\n")
	if err := os.WriteFile(filepath.Join(dir, "mine.yaml"), mine, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureDir(dir, st); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "mine.yaml")); string(got) != string(mine) {
		t.Errorf("mine.yaml = %q, want it untouched", got)
	}

	files := []string{filepath.Join(storeRoot, "pristine.yaml")}
	for _, name := range []string{"base", "go", "web", "flutter"} {
		files = append(files, filepath.Join(dir, name+".yaml"))
	}
	old := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	for _, f := range files {
		if err := os.Chtimes(f, old, old); err != nil {
			t.Fatal(err)
		}
	}

	if err := EnsureDir(dir, st); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		info, err := os.Stat(f)
		if err != nil {
			t.Fatal(err)
		}
		if !info.ModTime().Equal(old) {
			t.Errorf("%s was rewritten by a second EnsureDir", filepath.Base(f))
		}
	}
}
