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

func TestSystemFilesBuildIsSessionStartGuardAddGuardVocabAndGuardPlans(t *testing.T) {
	files := SystemFiles(ModeBuild)
	want := []string{".claude/hooks/session-start.sh", ".claude/hooks/guard-add.sh", ".claude/hooks/guard-vocab.sh", ".claude/hooks/guard-plans.sh"}
	if len(files) != len(want) {
		t.Fatalf("SystemFiles(build) = %v, want %v", files, want)
	}
	for i, p := range want {
		if files[i].Path != p {
			t.Fatalf("SystemFiles(build)[%d] = %q, want %q", i, files[i].Path, p)
		}
	}
}

func TestSystemFilesLearnAddsGuardAndRule(t *testing.T) {
	files := SystemFiles(ModeLearn)
	want := map[string]bool{
		".claude/hooks/session-start.sh":    false,
		".claude/hooks/guard-add.sh":        false,
		".claude/hooks/guard-vocab.sh":      false,
		".claude/hooks/guard-plans.sh":      false,
		".claude/rules/learn.md":            false,
		".claude/hooks/guard-code.sh":       false,
		".claude/rules/learning-journal.md": false,
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

func TestHookSettingsBuildHasGuardAddAndGuardPlansPreToolUse(t *testing.T) {
	settings := HookSettings(ModeBuild)
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("hooks missing or wrong type: %#v", settings)
	}
	if _, ok := hooks["SessionStart"]; !ok {
		t.Fatalf("hooks = %#v, want SessionStart", hooks)
	}
	pre, ok := hooks["PreToolUse"].([]any)
	if !ok {
		t.Fatalf("hooks = %#v, want PreToolUse in build mode (guard-add and guard-plans are unconditional)", hooks)
	}
	if len(pre) != 2 {
		t.Fatalf("PreToolUse = %#v, want the guard-add and guard-plans matchers in build mode", pre)
	}
	first, ok := pre[0].(map[string]any)
	if !ok || first["matcher"] != "Bash" {
		t.Fatalf("PreToolUse[0] = %#v, want matcher \"Bash\" (guard-add.sh)", pre[0])
	}
	second, ok := pre[1].(map[string]any)
	if !ok || second["matcher"] != "Write" {
		t.Fatalf("PreToolUse[1] = %#v, want matcher \"Write\" (guard-plans.sh)", pre[1])
	}
}

func TestHookSettingsLearnHasPreToolUse(t *testing.T) {
	settings := HookSettings(ModeLearn)
	hooks := settings["hooks"].(map[string]any)
	if _, ok := hooks["SessionStart"]; !ok {
		t.Fatalf("hooks = %#v, want SessionStart", hooks)
	}
	pre, ok := hooks["PreToolUse"].([]any)
	if !ok {
		t.Fatalf("hooks = %#v, want PreToolUse in learn mode", hooks)
	}
	if len(pre) != 3 {
		t.Fatalf("PreToolUse = %#v, want guard-add (Bash) plus guard-plans (Write) plus guard-code (Edit|Write|MultiEdit)", pre)
	}
	first, ok := pre[0].(map[string]any)
	if !ok || first["matcher"] != "Bash" {
		t.Fatalf("PreToolUse[0] = %#v, want matcher \"Bash\" (guard-add.sh)", pre[0])
	}
	second, ok := pre[1].(map[string]any)
	if !ok || second["matcher"] != "Write" {
		t.Fatalf("PreToolUse[1] = %#v, want matcher \"Write\" (guard-plans.sh)", pre[1])
	}
	third, ok := pre[2].(map[string]any)
	if !ok || third["matcher"] != "Edit|Write|MultiEdit" {
		t.Fatalf("PreToolUse[2] = %#v, want matcher \"Edit|Write|MultiEdit\" (guard-code.sh)", pre[2])
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

func runSessionStart(t *testing.T, scriptPath string) string {
	t.Helper()
	cmd := exec.Command("bash", scriptPath)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("session-start.sh failed: %v\noutput: %s", err, out.String())
	}
	return out.String()
}

func TestSessionStartCountsInjectedHandoffs(t *testing.T) {
	requireBashAndJQ(t)

	target := t.TempDir()
	if _, err := WriteFiles(SystemFiles(ModeBuild), Options{Target: target}); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(target, ".claude/hooks/session-start.sh")
	countPath := filepath.Join(target, ".claude/.ray-metrics/handoffs.count")

	if err := os.WriteFile(filepath.Join(target, ".claude/handoff.md"), []byte("state"), 0o644); err != nil {
		t.Fatal(err)
	}

	out1 := runSessionStart(t, scriptPath)
	if !strings.Contains(out1, "state") {
		t.Fatalf("session-start.sh output = %q, want it to include the handoff content", out1)
	}
	data, err := os.ReadFile(countPath)
	if err != nil {
		t.Fatalf("stat handoffs.count after 1st run: %v", err)
	}
	if strings.TrimSpace(string(data)) != "1" {
		t.Errorf("handoffs.count after 1st run = %q, want %q", data, "1")
	}

	runSessionStart(t, scriptPath)
	data, err = os.ReadFile(countPath)
	if err != nil {
		t.Fatalf("stat handoffs.count after 2nd run: %v", err)
	}
	if strings.TrimSpace(string(data)) != "2" {
		t.Errorf("handoffs.count after 2nd run = %q, want %q", data, "2")
	}
}

func TestSessionStartInjectsJournalHeadWhenPresent(t *testing.T) {
	requireBashAndJQ(t)

	target := t.TempDir()
	if _, err := WriteFiles(SystemFiles(ModeBuild), Options{Target: target}); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(target, ".claude/hooks/session-start.sh")

	journalDir := filepath.Join(target, ".claude/.local")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(journalDir, "learning-journal.md"), []byte("Milestones passed: 1/2"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runSessionStart(t, scriptPath)
	if !strings.Contains(out, "Milestones passed: 1/2") {
		t.Errorf("session-start.sh output = %q, want it to include the journal head", out)
	}
}

func TestSessionStartWithoutHandoffRecordsNoMetric(t *testing.T) {
	requireBashAndJQ(t)

	target := t.TempDir()
	if _, err := WriteFiles(SystemFiles(ModeBuild), Options{Target: target}); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(target, ".claude/hooks/session-start.sh")

	out := runSessionStart(t, scriptPath)
	if strings.TrimSpace(out) != "" {
		t.Errorf("session-start.sh output = %q, want empty with no handoff.md", out)
	}
	if _, err := os.Stat(filepath.Join(target, ".claude/.ray-metrics")); !os.IsNotExist(err) {
		t.Error(".ray-metrics/ should not exist: no handoff was injected, no activity to record")
	}
}
