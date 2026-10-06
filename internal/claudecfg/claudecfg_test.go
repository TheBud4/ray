package claudecfg

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
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

	if err := MergeSettings(target, settings, false, false, nil); err != nil {
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
	if err := MergeSettings(target, settings, false, false, nil); err != nil {
		t.Fatalf("MergeSettings() error = %v", err)
	}

	m := readSettingsJSON(t, target)
	if m["someUnmanagedKey"] != "keep-me" {
		t.Fatalf("someUnmanagedKey lost: %#v", m)
	}
	if m["model"] != "old-model" {
		t.Fatalf("model = %v, want the user's old-model kept without force", m["model"])
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

	if err := MergeSettings(target, settings, false, false, nil); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(target, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := MergeSettings(target, settings, false, false, nil); err != nil {
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

	if err := MergeSettings(target, settings, false, true, &out); err != nil {
		t.Fatalf("MergeSettings() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Fatalf("settings.json should not exist after dry-run, stat err = %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("dry-run should print the resulting JSON to out")
	}
}

func hookEntry(matcher, command string) map[string]any {
	e := map[string]any{"hooks": []any{map[string]any{"type": "command", "command": command}}}
	if matcher != "" {
		e["matcher"] = matcher
	}
	return e
}

func writeSettingsJSON(t *testing.T, target, content string) {
	t.Helper()
	dir := filepath.Join(target, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// eventCommands devolve, na ordem do arquivo, todo comando de hook de um evento.
func eventCommands(t *testing.T, m map[string]any, event string) []string {
	t.Helper()
	hooks, _ := m["hooks"].(map[string]any)
	entries, _ := hooks[event].([]any)
	var cmds []string
	for _, e := range entries {
		inner, _ := e.(map[string]any)["hooks"].([]any)
		for _, h := range inner {
			cmds = append(cmds, h.(map[string]any)["command"].(string))
		}
	}
	return cmds
}

// Sem --force, o que o usuário já escolheu vence; o que falta é preenchido.
func TestMergeSettingsKeepsUsersTopLevelKeysAndFillsMissingOnes(t *testing.T) {
	target := t.TempDir()
	writeSettingsJSON(t, target, `{"model": "sonnet"}`)

	if err := MergeSettings(target, map[string]any{"model": "opus", "effortLevel": "high"}, false, false, nil); err != nil {
		t.Fatal(err)
	}
	m := readSettingsJSON(t, target)
	if m["model"] != "sonnet" {
		t.Errorf("model = %v, want the user's sonnet", m["model"])
	}
	if m["effortLevel"] != "high" {
		t.Errorf("effortLevel = %v, want high filled in", m["effortLevel"])
	}
}

func TestMergeSettingsKeepsUsersHooksAndAddsTheRaysOwn(t *testing.T) {
	target := t.TempDir()
	writeSettingsJSON(t, target, `{"hooks": {
	  "Stop": [{"hooks": [{"type": "command", "command": "notify-send done"}]}],
	  "PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "my-bash-guard.sh"}]}]
	}}`)
	ray := map[string]any{"hooks": map[string]any{
		"SessionStart": []any{hookEntry("", "bash .claude/hooks/session-start.sh")},
		"PreToolUse":   []any{hookEntry("Bash", "bash .claude/hooks/guard-add.sh")},
	}}

	if err := MergeSettings(target, ray, false, false, nil); err != nil {
		t.Fatal(err)
	}
	m := readSettingsJSON(t, target)
	if got := eventCommands(t, m, "Stop"); !slices.Equal(got, []string{"notify-send done"}) {
		t.Errorf("Stop = %v, want the user's hook untouched", got)
	}
	if got := eventCommands(t, m, "PreToolUse"); !slices.Equal(got, []string{"my-bash-guard.sh", "bash .claude/hooks/guard-add.sh"}) {
		t.Errorf("PreToolUse = %v, want the user's guard then the ray's", got)
	}
	if got := eventCommands(t, m, "SessionStart"); !slices.Equal(got, []string{"bash .claude/hooks/session-start.sh"}) {
		t.Errorf("SessionStart = %v, want the ray's hook added", got)
	}
}

// Um hook do ray com nome antigo (a versão nova não o traz mais) é retirado, e o
// do usuário fica; rodar de novo não muda o arquivo.
func TestMergeSettingsReplacesTheRaysOwnHooksAndIsIdempotent(t *testing.T) {
	target := t.TempDir()
	writeSettingsJSON(t, target, `{"hooks": {"SessionStart": [
	  {"hooks": [{"type": "command", "command": "bash .claude/hooks/antigo.sh"}]},
	  {"hooks": [{"type": "command", "command": "bash meu-script.sh"}]}
	]}}`)
	ray := map[string]any{"hooks": map[string]any{
		"SessionStart": []any{hookEntry("", "bash .claude/hooks/session-start.sh")},
	}}

	if err := MergeSettings(target, ray, false, false, nil); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(filepath.Join(target, ".claude", "settings.json"))
	got := eventCommands(t, readSettingsJSON(t, target), "SessionStart")
	if want := []string{"bash meu-script.sh", "bash .claude/hooks/session-start.sh"}; !slices.Equal(got, want) {
		t.Errorf("SessionStart = %v, want %v", got, want)
	}

	if err := MergeSettings(target, ray, false, false, nil); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(filepath.Join(target, ".claude", "settings.json"))
	if !bytes.Equal(first, second) {
		t.Errorf("2nd application changed the file:\n--- 1st ---\n%s\n--- 2nd ---\n%s", first, second)
	}
}

func TestMergeSettingsForceReplacesHooksAndTopLevelKeys(t *testing.T) {
	target := t.TempDir()
	writeSettingsJSON(t, target, `{"model": "sonnet", "keep": "me", "hooks": {"Stop": [{"hooks": [{"type": "command", "command": "notify-send done"}]}]}}`)
	ray := map[string]any{
		"model": "opus",
		"hooks": map[string]any{"SessionStart": []any{hookEntry("", "bash .claude/hooks/session-start.sh")}},
	}

	if err := MergeSettings(target, ray, true, false, nil); err != nil {
		t.Fatal(err)
	}
	m := readSettingsJSON(t, target)
	if m["model"] != "opus" {
		t.Errorf("model = %v, want opus under force", m["model"])
	}
	if m["keep"] != "me" {
		t.Errorf("keep = %v, want unmanaged keys preserved even under force", m["keep"])
	}
	if got := eventCommands(t, m, "Stop"); len(got) != 0 {
		t.Errorf("Stop = %v, want the user's hooks replaced under force", got)
	}
	if got := eventCommands(t, m, "SessionStart"); !slices.Equal(got, []string{"bash .claude/hooks/session-start.sh"}) {
		t.Errorf("SessionStart = %v", got)
	}
}

// `null` é JSON válido e Unmarshal o entrega como mapa nil: gravar nele dava
// panic. Um documento nulo não tem nada a preservar, então vale como vazio.
func TestMergeSettingsTreatsANullDocumentAsEmpty(t *testing.T) {
	target := t.TempDir()
	writeSettingsJSON(t, target, "null\n")

	if err := MergeSettings(target, map[string]any{"model": "opus"}, false, false, nil); err != nil {
		t.Fatalf("MergeSettings() error = %v", err)
	}
	if m := readSettingsJSON(t, target); m["model"] != "opus" {
		t.Errorf("settings = %v, want the merged model", m)
	}
}
