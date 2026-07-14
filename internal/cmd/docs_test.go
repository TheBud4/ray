package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/rayconfig"
	"github.com/TheBud4/ray/internal/runner"
)

func TestRunDocsInitCreatesLayoutAndConfigures(t *testing.T) {
	base := t.TempDir()
	configPath := filepath.Join(base, "config.yaml")
	vaultPath := filepath.Join(base, "docs-vault")

	if err := runDocsInit(configPath, vaultPath); err != nil {
		t.Fatalf("runDocsInit() error = %v", err)
	}

	for _, sub := range []string{"guides", "concepts", ".obsidian", "README.md"} {
		if _, err := os.Stat(filepath.Join(vaultPath, sub)); err != nil {
			t.Errorf("stat %s: %v", sub, err)
		}
	}

	cfg, err := rayconfig.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UserDocsVault != vaultPath {
		t.Errorf("UserDocsVault = %q, want %q", cfg.UserDocsVault, vaultPath)
	}
}

func TestRunDocsSetPointsToExistingDirWithoutReorganizing(t *testing.T) {
	base := t.TempDir()
	configPath := filepath.Join(base, "config.yaml")
	existing := filepath.Join(base, "my-obsidian-vault")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(existing, "already-here.md")
	if err := os.WriteFile(marker, []byte("pre-existing"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runDocsSet(configPath, existing); err != nil {
		t.Fatalf("runDocsSet() error = %v", err)
	}

	if _, err := os.Stat(filepath.Join(existing, "guides")); !os.IsNotExist(err) {
		t.Errorf("runDocsSet should not create guides/, stat err = %v", err)
	}
	got, err := os.ReadFile(marker)
	if err != nil || string(got) != "pre-existing" {
		t.Errorf("pre-existing content disturbed: got %q, err %v", got, err)
	}

	cfg, err := rayconfig.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UserDocsVault != existing {
		t.Errorf("UserDocsVault = %q, want %q", cfg.UserDocsVault, existing)
	}
}

func TestRunDocsSetErrorsOnMissingDir(t *testing.T) {
	base := t.TempDir()
	configPath := filepath.Join(base, "config.yaml")

	err := runDocsSet(configPath, filepath.Join(base, "does-not-exist"))
	if err == nil {
		t.Fatal("runDocsSet() = nil error, want error for a nonexistent path")
	}
}

func TestRunDocsOpenAndPathRequireConfiguration(t *testing.T) {
	base := t.TempDir()
	configPath := filepath.Join(base, "config.yaml")

	if _, err := resolveDocsPath(configPath); err == nil {
		t.Fatal("resolveDocsPath() = nil error, want error when unconfigured")
	}
	if err := runDocsOpen(&runner.FakeRunner{}, configPath); err == nil {
		t.Fatal("runDocsOpen() = nil error, want error when unconfigured")
	}
}

func TestRunDocsOpenAfterConfiguring(t *testing.T) {
	base := t.TempDir()
	configPath := filepath.Join(base, "config.yaml")
	vaultPath := filepath.Join(base, "docs-vault")
	if err := runDocsInit(configPath, vaultPath); err != nil {
		t.Fatal(err)
	}

	fr := &runner.FakeRunner{}
	if err := runDocsOpen(fr, configPath); err != nil {
		t.Fatalf("runDocsOpen() error = %v", err)
	}
	if len(fr.Calls) != 1 || !strings.Contains(fr.Calls[0].String(), vaultPath) {
		t.Fatalf("Calls = %v, want a call referencing %q", fr.Calls, vaultPath)
	}
}
