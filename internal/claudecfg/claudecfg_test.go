package claudecfg

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func readSettingsJSON(t *testing.T, target string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(target, ".claude", "settings.json"))
	if err != nil {
		t.Fatalf("reading settings.json: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parsing settings.json: %v", err)
	}
	return m
}

func TestMergeSettingsCreatesFromScratch(t *testing.T) {
	target := t.TempDir()
	settings := map[string]any{"model": "opus", "effortLevel": "high"}

	if err := MergeSettings(target, settings, false, nil); err != nil {
		t.Fatalf("MergeSettings() error = %v", err)
	}

	m := readSettingsJSON(t, target)
	if m["model"] != "opus" {
		t.Fatalf("model = %v, want opus", m["model"])
	}
	if m["effortLevel"] != "high" {
		t.Fatalf("effortLevel = %v, want high", m["effortLevel"])
	}
}

func TestMergeSettingsPreservesUnmanagedKeys(t *testing.T) {
	target := t.TempDir()
	claudeDir := filepath.Join(target, ".claude")
	if err := os.MkdirAll(claudeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := `{"someUnmanagedKey": "keep-me", "model": "old-model"}`
	if err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	settings := map[string]any{"model": "opus"}
	if err := MergeSettings(target, settings, false, nil); err != nil {
		t.Fatalf("MergeSettings() error = %v", err)
	}

	m := readSettingsJSON(t, target)
	if m["someUnmanagedKey"] != "keep-me" {
		t.Fatalf("someUnmanagedKey lost: %#v", m)
	}
	if m["model"] != "opus" {
		t.Fatalf("model = %v, want opus (overwritten)", m["model"])
	}
}

func TestMergeSettingsIsIdempotent(t *testing.T) {
	target := t.TempDir()
	settings := map[string]any{
		"model":       "opus",
		"effortLevel": "high",
		"hooks": map[string]any{
			"SessionStart": []any{map[string]any{"hooks": []any{map[string]any{"command": ".claude/hooks/session-start.sh"}}}},
		},
	}

	if err := MergeSettings(target, settings, false, nil); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(target, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := MergeSettings(target, settings, false, nil); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(target, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("2nd application changed the file:\n--- 1st ---\n%s\n--- 2nd ---\n%s", first, second)
	}
	if !bytes.HasSuffix(second, []byte("\n")) {
		t.Fatalf("file does not end with newline: %q", second)
	}
}

func TestMergeSettingsDryRunDoesNotWrite(t *testing.T) {
	target := t.TempDir()
	var out bytes.Buffer
	settings := map[string]any{"model": "opus"}

	if err := MergeSettings(target, settings, true, &out); err != nil {
		t.Fatalf("MergeSettings() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Fatalf("settings.json should not exist after dry-run, stat err = %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("dry-run should print the resulting JSON to out")
	}
}
