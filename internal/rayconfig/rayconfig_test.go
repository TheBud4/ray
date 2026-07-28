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
	if cfg.Brain != "" {
		t.Errorf("Brain = %q, want empty", cfg.Brain)
	}
}

func TestConfigSaveAndLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	cfg := &Config{Brain: "/home/u/Docs"}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.Brain != "/home/u/Docs" {
		t.Errorf("Brain = %q, want %q", got.Brain, "/home/u/Docs")
	}
}

func TestSetBrainPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")

	cfg := &Config{}
	if err := cfg.SetBrain(path, "/vault"); err != nil {
		t.Fatalf("SetBrain() error = %v", err)
	}

	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Brain != "/vault" {
		t.Errorf("Brain = %q, want %q", got.Brain, "/vault")
	}
}

func TestBrainPathPrefersEnvOverride(t *testing.T) {
	cfg := &Config{Brain: "/from/config"}

	t.Setenv("RAY_BRAIN", "")
	if got := cfg.BrainPath(); got != "/from/config" {
		t.Errorf("BrainPath() = %q, want config value %q", got, "/from/config")
	}

	t.Setenv("RAY_BRAIN", "/from/env")
	if got := cfg.BrainPath(); got != "/from/env" {
		t.Errorf("BrainPath() = %q, want env override %q", got, "/from/env")
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

	cfg := &Config{Brain: "/x"}
	if err := cfg.Save(path); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat %s: %v", path, err)
	}
}
