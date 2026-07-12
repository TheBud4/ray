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

	t.Run("skips broken yaml", func(t *testing.T) {
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
		if len(entries) != 3 {
			t.Errorf("List() returned %d entries, want 3 (broken.yaml should be skipped)", len(entries))
		}
	})
}
