package vault

import (
	"os"
	"path/filepath"
	"testing"
)

func TestVerifyAcceptsAnyExistingDir(t *testing.T) {
	dir := t.TempDir()

	// Sem layout imposto: uma pasta vazia já é um cérebro válido.
	if err := Verify(dir); err != nil {
		t.Fatalf("Verify() error = %v, want nil for an existing directory", err)
	}
}

func TestVerifyNeverCreatesAnything(t *testing.T) {
	base := t.TempDir()
	missing := filepath.Join(base, "brain")

	if err := Verify(missing); err == nil {
		t.Fatal("Verify() = nil, want error for a missing path")
	}

	// A garantia central do refactor: o ray é consumidor, não dono.
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatalf("Verify() created %s; it must never write to disk", missing)
	}
	entries, err := os.ReadDir(base)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("Verify() left %d entries in the parent dir, want 0", len(entries))
	}
}

func TestVerifyRejectsEmptyPath(t *testing.T) {
	if err := Verify(""); err == nil {
		t.Fatal("Verify(\"\") = nil, want error")
	}
}

func TestVerifyRejectsNonDirectory(t *testing.T) {
	file := filepath.Join(t.TempDir(), "brain.md")
	if err := os.WriteFile(file, []byte("# não é uma pasta"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Verify(file); err == nil {
		t.Fatal("Verify() = nil for a regular file, want error")
	}
}

func TestStatMissingDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")

	st, err := Stat(dir)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if st.Exists {
		t.Fatalf("Exists = true, want false")
	}
	if st.MarkdownCount != 0 {
		t.Fatalf("MarkdownCount = %d, want 0", st.MarkdownCount)
	}
	if st.Path != dir {
		t.Fatalf("Path = %q, want %q", st.Path, dir)
	}
}

func TestStatCountsMarkdownRecursively(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "Projetos", "Trabalho")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "Home.md"), []byte("# home"), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err := Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !st.Exists {
		t.Fatalf("Exists = false, want true")
	}
	if st.MarkdownCount != 1 {
		t.Fatalf("MarkdownCount = %d, want 1", st.MarkdownCount)
	}

	if err := os.WriteFile(filepath.Join(nested, "nota.md"), []byte("# nota"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "imagem.png"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err = Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if st.MarkdownCount != 2 {
		t.Fatalf("MarkdownCount = %d, want 2 (nested .md counted, .png ignored)", st.MarkdownCount)
	}
}
