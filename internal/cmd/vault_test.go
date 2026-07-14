package cmd

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/vault"
)

func TestRunVaultStatusOnMissingDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "does-not-exist")

	var out bytes.Buffer
	if err := runVaultStatus(dir, &out); err != nil {
		t.Fatalf("runVaultStatus() error = %v", err)
	}
	if !strings.Contains(out.String(), "exists: no") {
		t.Errorf("output = %q, want it to report the vault does not exist", out.String())
	}
}

func TestRunVaultStatusAfterInit(t *testing.T) {
	dir := t.TempDir()
	if err := vault.Ensure(dir); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runVaultStatus(dir, &out); err != nil {
		t.Fatalf("runVaultStatus() error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "exists: yes") {
		t.Errorf("output = %q, want it to report the vault exists", got)
	}
	if !strings.Contains(got, "markdown files: 1") {
		t.Errorf("output = %q, want markdown files: 1 (README.md)", got)
	}
}

func TestRunVaultOpenRunsPlatformCommand(t *testing.T) {
	fr := &runner.FakeRunner{}
	dir := "/some/vault"

	if err := runVaultOpen(fr, dir); err != nil {
		t.Fatalf("runVaultOpen() error = %v", err)
	}
	if len(fr.Calls) != 1 {
		t.Fatalf("Calls = %v, want exactly 1 call", fr.Calls)
	}
}
