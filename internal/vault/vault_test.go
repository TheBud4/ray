package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureCreatesLayout(t *testing.T) {
	dir := t.TempDir()
	vaultDir := filepath.Join(dir, "vault")

	if err := Ensure(vaultDir); err != nil {
		t.Fatalf("Ensure() error = %v", err)
	}

	for _, sub := range []string{"inbox", "notes", ".obsidian"} {
		info, err := os.Stat(filepath.Join(vaultDir, sub))
		if err != nil {
			t.Fatalf("stat %s: %v", sub, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", sub)
		}
	}
	if _, err := os.Stat(filepath.Join(vaultDir, "README.md")); err != nil {
		t.Fatalf("stat README.md: %v", err)
	}
}

func TestEnsureIsIdempotentAndNeverOverwrites(t *testing.T) {
	dir := t.TempDir()
	vaultDir := filepath.Join(dir, "vault")

	if err := Ensure(vaultDir); err != nil {
		t.Fatal(err)
	}

	readmePath := filepath.Join(vaultDir, "README.md")
	custom := []byte("meu conteúdo customizado")
	if err := os.WriteFile(readmePath, custom, 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Ensure(vaultDir); err != nil {
		t.Fatalf("2nd Ensure() error = %v", err)
	}

	got, err := os.ReadFile(readmePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(custom) {
		t.Fatalf("README.md overwritten: got %q, want %q", got, custom)
	}
}

func TestStatMissingDir(t *testing.T) {
	dir := t.TempDir()
	vaultDir := filepath.Join(dir, "does-not-exist")

	st, err := Stat(vaultDir)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if st.Exists {
		t.Fatalf("Exists = true, want false")
	}
	if st.MarkdownCount != 0 {
		t.Fatalf("MarkdownCount = %d, want 0", st.MarkdownCount)
	}
	if st.Path != vaultDir {
		t.Fatalf("Path = %q, want %q", st.Path, vaultDir)
	}
}

func TestStatCountsMarkdownRecursively(t *testing.T) {
	dir := t.TempDir()
	vaultDir := filepath.Join(dir, "vault")
	if err := Ensure(vaultDir); err != nil {
		t.Fatal(err)
	}

	st, err := Stat(vaultDir)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Exists {
		t.Fatalf("Exists = false, want true")
	}
	if st.MarkdownCount != 1 {
		t.Fatalf("MarkdownCount = %d, want 1 (README.md)", st.MarkdownCount)
	}

	if err := os.WriteFile(filepath.Join(vaultDir, "notes", "note.md"), []byte("# nota"), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err = Stat(vaultDir)
	if err != nil {
		t.Fatal(err)
	}
	if st.MarkdownCount != 2 {
		t.Fatalf("MarkdownCount = %d, want 2 after adding a nested note", st.MarkdownCount)
	}
}
