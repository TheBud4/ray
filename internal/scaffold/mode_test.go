package scaffold

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemFilesBuildIsJustSessionStart(t *testing.T) {
	files := SystemFiles(ModeBuild)
	if len(files) != 1 || files[0].Path != ".claude/hooks/session-start.sh" {
		t.Fatalf("SystemFiles(build) = %v, want just session-start.sh", files)
	}
}

func TestSystemFilesLearnAddsGuardAndRule(t *testing.T) {
	files := SystemFiles(ModeLearn)
	want := map[string]bool{
		".claude/hooks/session-start.sh": false,
		".claude/rules/learn.md":         false,
		".claude/hooks/guard-code.sh":    false,
	}
	if len(files) != len(want) {
		t.Fatalf("SystemFiles(learn) = %v, want %d entries", files, len(want))
	}
	for _, f := range files {
		if _, ok := want[f.Path]; !ok {
			t.Fatalf("unexpected path %q in SystemFiles(learn)", f.Path)
		}
		want[f.Path] = true
	}
	for p, seen := range want {
		if !seen {
			t.Fatalf("SystemFiles(learn) missing %q", p)
		}
	}
}

func TestHookSettingsBuildHasNoPreToolUse(t *testing.T) {
	settings := HookSettings(ModeBuild)
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("hooks missing or wrong type: %#v", settings)
	}
	if _, ok := hooks["SessionStart"]; !ok {
		t.Fatalf("hooks = %#v, want SessionStart", hooks)
	}
	if _, ok := hooks["PreToolUse"]; ok {
		t.Fatalf("hooks = %#v, want no PreToolUse in build mode", hooks)
	}
}

func TestHookSettingsLearnHasPreToolUse(t *testing.T) {
	settings := HookSettings(ModeLearn)
	hooks := settings["hooks"].(map[string]any)
	if _, ok := hooks["SessionStart"]; !ok {
		t.Fatalf("hooks = %#v, want SessionStart", hooks)
	}
	if _, ok := hooks["PreToolUse"]; !ok {
		t.Fatalf("hooks = %#v, want PreToolUse in learn mode", hooks)
	}
}

func requireBashAndJQ(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not available")
	}
}

func runGuardCode(t *testing.T, scriptPath, filePath string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"tool_input": map[string]any{"file_path": filePath},
	})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", scriptPath)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Dir = filepath.Dir(scriptPath)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("guard-code.sh failed: %v\noutput: %s", err, out.String())
	}
	return out.String()
}

func TestGuardCodeAllowsDocsBlocksCode(t *testing.T) {
	requireBashAndJQ(t)

	target := t.TempDir()
	if _, err := WriteFiles(SystemFiles(ModeLearn), Options{Target: target}); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(target, ".claude/hooks/guard-code.sh")

	if err := os.MkdirAll(filepath.Join(target, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(target, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}

	docsOut := runGuardCode(t, scriptPath, filepath.Join(target, "docs/x.md"))
	if strings.TrimSpace(docsOut) != "" {
		t.Fatalf("guard-code.sh on docs/x.md = %q, want empty (allowed)", docsOut)
	}

	codeOut := runGuardCode(t, scriptPath, filepath.Join(target, "lib/main.dart"))
	if !strings.Contains(codeOut, "block") {
		t.Fatalf("guard-code.sh on lib/main.dart = %q, want it to contain \"block\"", codeOut)
	}
}
