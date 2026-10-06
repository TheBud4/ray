package fsutil

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestWriteFileAtomicCreatesAndReplaces(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yaml")

	if err := WriteFileAtomic(path, []byte("one"), 0o644); err != nil {
		t.Fatalf("WriteFileAtomic() error = %v", err)
	}
	if err := WriteFileAtomic(path, []byte("two"), 0o644); err != nil {
		t.Fatalf("WriteFileAtomic() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "two" {
		t.Errorf("content = %q, want the second write", got)
	}
}

// Sem sobra: nem depois do sucesso nem depois de uma falha. Um temporário
// esquecido ao lado de ~/.ray/config.yaml é lixo que ninguém sabe se pode apagar.
func TestWriteFileAtomicLeavesNoTemporaryBehind(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")
	if err := WriteFileAtomic(path, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Falha: o destino é um diretório, e o rename por cima dele não é possível.
	blocked := filepath.Join(dir, "blocked")
	if err := os.MkdirAll(filepath.Join(blocked, "inner"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(blocked, []byte("x"), 0o644); err == nil {
		t.Fatal("WriteFileAtomic() over a non-empty directory = nil error, want a failure")
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "state.yaml" && e.Name() != "blocked" {
			t.Errorf("stray entry %q left in the directory", e.Name())
		}
	}
	if _, err := os.Stat(filepath.Join(blocked, "inner")); err != nil {
		t.Errorf("the destination was disturbed by the failed write: %v", err)
	}
}

func TestWriteFileAtomicFailureKeepsTheOriginal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.yaml")
	if err := os.WriteFile(path, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := WriteFileAtomic(filepath.Join(dir, "missing-dir", "state.yaml"), []byte("new"), 0o644)
	if err == nil {
		t.Fatal("WriteFileAtomic() into a missing directory = nil error, want a failure")
	}
	if got, _ := os.ReadFile(path); string(got) != "original" {
		t.Errorf("original = %q, want it untouched", got)
	}
}

func TestWriteFileAtomicAppliesTheRequestedMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("modos POSIX")
	}
	path := filepath.Join(t.TempDir(), "state.yaml")
	if err := WriteFileAtomic(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Errorf("mode = %o, want 644", got)
	}
}
