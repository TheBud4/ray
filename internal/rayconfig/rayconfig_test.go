package rayconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingConfigIsZeroValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v, want nil for missing file", err)
	}
	if cfg.UserDocsVault != "" {
		t.Errorf("UserDocsVault = %q, want empty", cfg.UserDocsVault)
	}
}

func TestConfigSaveAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	cfg := &Config{UserDocsVault: "/home/u/Docs"}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.UserDocsVault != "/home/u/Docs" {
		t.Errorf("UserDocsVault = %q, want %q", got.UserDocsVault, "/home/u/Docs")
	}
}

func TestSetUserDocsVaultPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	cfg := &Config{}
	if err := cfg.SetUserDocsVault(path, "/vault"); err != nil {
		t.Fatalf("SetUserDocsVault() error = %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.UserDocsVault != "/vault" {
		t.Errorf("UserDocsVault = %q, want %q", got.UserDocsVault, "/vault")
	}
}

func TestUserDocsVaultPathPrefersEnvOverride(t *testing.T) {
	cfg := &Config{UserDocsVault: "/from/config"}

	t.Setenv("RAY_DOCS_VAULT", "")
	if got := cfg.UserDocsVaultPath(); got != "/from/config" {
		t.Errorf("UserDocsVaultPath() = %q, want config value %q", got, "/from/config")
	}

	t.Setenv("RAY_DOCS_VAULT", "/from/env")
	if got := cfg.UserDocsVaultPath(); got != "/from/env" {
		t.Errorf("UserDocsVaultPath() = %q, want env override %q", got, "/from/env")
	}
}

func TestLoadStateMissingIsZeroValue(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yaml")

	st, err := LoadState(path)
	if err != nil {
		t.Fatalf("LoadState() error = %v, want nil for missing file", err)
	}
	if len(st.InstalledGlobals) != 0 {
		t.Errorf("InstalledGlobals = %v, want empty", st.InstalledGlobals)
	}
	if st.HasGlobal("headroom") {
		t.Error("HasGlobal(headroom) = true, want false on empty state")
	}
}

func TestStateSaveAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.yaml")

	st := &State{}
	st.AddGlobal("headroom")
	if err := st.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := LoadState(path)
	if err != nil {
		t.Fatal(err)
	}
	if !got.HasGlobal("headroom") {
		t.Error("HasGlobal(headroom) = false after round-trip, want true")
	}
}

func TestAddGlobalIsIdempotent(t *testing.T) {
	st := &State{}
	st.AddGlobal("headroom")
	st.AddGlobal("headroom")
	st.AddGlobal("code_graph")

	if len(st.InstalledGlobals) != 2 {
		t.Fatalf("InstalledGlobals = %v, want exactly 2 entries (no duplicate)", st.InstalledGlobals)
	}
	if !st.HasGlobal("headroom") || !st.HasGlobal("code_graph") {
		t.Fatalf("InstalledGlobals = %v, want both headroom and code_graph", st.InstalledGlobals)
	}
}

func TestConfigSaveCreatesParentDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "dir", "config.yaml")

	cfg := &Config{UserDocsVault: "/x"}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
}
