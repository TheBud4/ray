package fsutil

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
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

// No Windows o rename falha com "acesso negado" enquanto um leitor concorrente
// tem o destino aberto. Um erro transitório é repetido até passar; o teste
// troca o rename e a classificação por falsos para valer em qualquer SO.
func TestWriteFileAtomicRetriesATransientRenameFailure(t *testing.T) {
	errBusy := errors.New("destination busy")
	calls := 0
	var slept []time.Duration
	origRename, origTransient, origSleep := renameFile, isTransientRename, sleepBetweenRetries
	t.Cleanup(func() { renameFile, isTransientRename, sleepBetweenRetries = origRename, origTransient, origSleep })
	renameFile = func(from, to string) error {
		calls++
		if calls <= 2 {
			return errBusy
		}
		return os.Rename(from, to)
	}
	isTransientRename = func(err error) bool { return errors.Is(err, errBusy) }
	sleepBetweenRetries = func(d time.Duration) { slept = append(slept, d) }

	path := filepath.Join(t.TempDir(), "state.yaml")
	if err := WriteFileAtomic(path, []byte("ok"), 0o644); err != nil {
		t.Fatalf("WriteFileAtomic() error = %v, want the transient failures absorbed", err)
	}
	if got, _ := os.ReadFile(path); string(got) != "ok" {
		t.Errorf("content = %q, want the written data", got)
	}
	if calls != 3 {
		t.Errorf("rename calls = %d, want 3 (two failures, then success)", calls)
	}
	if len(slept) != 2 || slept[1] <= slept[0] {
		t.Errorf("sleeps = %v, want two growing waits", slept)
	}
}

// A espera é limitada: um destino que nunca libera falha com o último erro, sem
// sobra de temporário, em vez de travar o comando.
func TestWriteFileAtomicGivesUpOnAPersistentTransientFailure(t *testing.T) {
	errBusy := errors.New("destination busy")
	calls := 0
	origRename, origTransient, origSleep := renameFile, isTransientRename, sleepBetweenRetries
	t.Cleanup(func() { renameFile, isTransientRename, sleepBetweenRetries = origRename, origTransient, origSleep })
	renameFile = func(string, string) error { calls++; return errBusy }
	isTransientRename = func(err error) bool { return errors.Is(err, errBusy) }
	sleepBetweenRetries = func(time.Duration) {}

	dir := t.TempDir()
	err := WriteFileAtomic(filepath.Join(dir, "state.yaml"), []byte("x"), 0o644)
	if !errors.Is(err, errBusy) {
		t.Fatalf("WriteFileAtomic() error = %v, want the last rename error", err)
	}
	if calls != renameAttempts {
		t.Errorf("rename calls = %d, want %d", calls, renameAttempts)
	}
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("directory has %d entries after the failure, want none", len(entries))
	}
}

// Um erro que não é transitório não é repetido: o comportamento de sempre.
func TestWriteFileAtomicDoesNotRetryAPermanentRenameFailure(t *testing.T) {
	calls := 0
	origRename, origSleep := renameFile, sleepBetweenRetries
	t.Cleanup(func() { renameFile, sleepBetweenRetries = origRename, origSleep })
	renameFile = func(string, string) error { calls++; return errors.New("permanent") }
	sleepBetweenRetries = func(time.Duration) { t.Error("slept on a permanent failure") }

	if err := WriteFileAtomic(filepath.Join(t.TempDir(), "state.yaml"), []byte("x"), 0o644); err == nil {
		t.Fatal("WriteFileAtomic() = nil, want the rename error")
	}
	if calls != 1 {
		t.Errorf("rename calls = %d, want 1", calls)
	}
}
