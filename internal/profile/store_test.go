package profile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureDirCreatesDefaults(t *testing.T) {
	dir := t.TempDir()

	if err := EnsureDir(dir); err != nil {
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

	if err := EnsureDir(dir); err != nil {
		t.Fatal(err)
	}

	goPath := filepath.Join(dir, "go.yaml")
	edited := "name: go\ndescription: EDITED\n"
	if err := os.WriteFile(goPath, []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureDir(dir); err != nil {
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
	if len(entries) != 3 {
		t.Errorf("dir has %d entries, want 3 (go/web/flutter)", len(entries))
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
		if err := EnsureDir(dir); err != nil {
			t.Fatal(err)
		}

		entries, err := List(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 3 {
			t.Fatalf("List() returned %d entries, want 3", len(entries))
		}
		names := []string{entries[0].Name, entries[1].Name, entries[2].Name}
		want := []string{"flutter", "go", "web"}
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
		if err := EnsureDir(dir); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "broken.yaml"), []byte(":\n  - ["), 0o644); err != nil {
			t.Fatal(err)
		}

		entries, err := List(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 4 {
			t.Fatalf("List() returned %d entries, want 4 (broken.yaml included)", len(entries))
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
		if err := EnsureDir(dir); err != nil {
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
