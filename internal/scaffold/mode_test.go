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

func TestSystemFilesIsSessionStartThreeWarningGuardsAndGuardHandoff(t *testing.T) {
	files := SystemFiles()
	want := []string{".claude/hooks/session-start.sh", ".claude/hooks/guard-add.sh", ".claude/hooks/guard-vocab.sh", ".claude/hooks/guard-plans.sh", ".claude/hooks/guard-handoff.sh"}
	if len(files) != len(want) {
		t.Fatalf("SystemFiles() = %v, want %v", files, want)
	}
	for i, p := range want {
		if files[i].Path != p {
			t.Fatalf("SystemFiles()[%d] = %q, want %q", i, files[i].Path, p)
		}
	}
}

func TestHookSettingsHasThreeWarningGuardsInPreToolUse(t *testing.T) {
	settings := HookSettings()
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("hooks missing or wrong type: %#v", settings)
	}
	if _, ok := hooks["SessionStart"]; !ok {
		t.Fatalf("hooks = %#v, want SessionStart", hooks)
	}
	pre, ok := hooks["PreToolUse"].([]any)
	if !ok {
		t.Fatalf("hooks = %#v, want PreToolUse (the three warning guards are unconditional)", hooks)
	}
	if len(pre) != 3 {
		t.Fatalf("PreToolUse = %#v, want guard-add, guard-plans and guard-vocab", pre)
	}
	first, ok := pre[0].(map[string]any)
	if !ok || first["matcher"] != "Bash" {
		t.Fatalf("PreToolUse[0] = %#v, want matcher \"Bash\" (guard-add.sh)", pre[0])
	}
	second, ok := pre[1].(map[string]any)
	if !ok || second["matcher"] != "Edit|Write|MultiEdit" {
		t.Fatalf("PreToolUse[1] = %#v, want matcher \"Edit|Write|MultiEdit\" (guard-plans.sh)", pre[1])
	}
	third, ok := pre[2].(map[string]any)
	if !ok || third["matcher"] != "Edit|Write|MultiEdit" {
		t.Fatalf("PreToolUse[2] = %#v, want matcher \"Edit|Write|MultiEdit\" (guard-vocab.sh)", pre[2])
	}

	// guard-handoff é o único guard que precisa do arquivo já no disco — Edit e
	// MultiEdit não carregam o conteúdo final no payload, só o trecho alterado
	// — por isso ele é PostToolUse e os três de cima continuam PreToolUse.
	post, ok := hooks["PostToolUse"].([]any)
	if !ok {
		t.Fatalf("hooks = %#v, want PostToolUse (guard-handoff is unconditional)", hooks)
	}
	if len(post) != 1 {
		t.Fatalf("PostToolUse = %#v, want only guard-handoff", post)
	}
	postFirst, ok := post[0].(map[string]any)
	if !ok || postFirst["matcher"] != "Write|Edit|MultiEdit" {
		t.Fatalf("PostToolUse[0] = %#v, want matcher \"Write|Edit|MultiEdit\" (guard-handoff.sh)", post[0])
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
	if _, err := WriteFiles(SystemFiles(), Options{Target: target}); err != nil {
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

func TestSessionStartWithoutHandoffRecordsNoMetric(t *testing.T) {
	requireBashAndJQ(t)

	target := t.TempDir()
	if _, err := WriteFiles(SystemFiles(), Options{Target: target}); err != nil {
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

// runGuardHandoff simula o PostToolUse: escreve n linhas em .claude/handoff.md
// dentro de target e roda o hook com o payload que o Claude Code manda depois
// de um Write real (file_path absoluto).
func runGuardHandoff(t *testing.T, scriptPath, target string, lines int) string {
	t.Helper()
	handoffPath := filepath.Join(target, ".claude/handoff.md")
	content := strings.Repeat("linha\n", lines)
	if err := os.WriteFile(handoffPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"tool_input": map[string]any{"file_path": handoffPath},
	})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", scriptPath)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Dir = target
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("guard-handoff.sh failed: %v\noutput: %s", err, out.String())
	}
	return out.String()
}

func TestGuardHandoffSilentUnderBudget(t *testing.T) {
	requireBashAndJQ(t)

	target := t.TempDir()
	if _, err := WriteFiles(SystemFiles(), Options{Target: target}); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(target, ".claude/hooks/guard-handoff.sh")

	out := runGuardHandoff(t, scriptPath, target, 40)
	if strings.TrimSpace(out) != "" {
		t.Errorf("guard-handoff.sh on a 40-line handoff = %q, want silent (on target)", out)
	}
}

func TestGuardHandoffWarnsOverBudget(t *testing.T) {
	requireBashAndJQ(t)

	target := t.TempDir()
	if _, err := WriteFiles(SystemFiles(), Options{Target: target}); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(target, ".claude/hooks/guard-handoff.sh")

	out := runGuardHandoff(t, scriptPath, target, 355)
	if !strings.Contains(out, "355") {
		t.Errorf("guard-handoff.sh on a 355-line handoff = %q, want the real line count in the message", out)
	}
	if !strings.Contains(out, "systemMessage") {
		t.Errorf("guard-handoff.sh output = %q, want a systemMessage, never a block", out)
	}
}

func TestGuardHandoffIgnoresOtherFiles(t *testing.T) {
	requireBashAndJQ(t)

	target := t.TempDir()
	if _, err := WriteFiles(SystemFiles(), Options{Target: target}); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(target, ".claude/hooks/guard-handoff.sh")

	otherPath := filepath.Join(target, "docs/architecture.md")
	if err := os.MkdirAll(filepath.Dir(otherPath), 0o755); err != nil {
		t.Fatal(err)
	}
	content := strings.Repeat("linha\n", 500)
	if err := os.WriteFile(otherPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{
		"tool_input": map[string]any{"file_path": otherPath},
	})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", scriptPath)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Dir = target
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("guard-handoff.sh failed: %v\noutput: %s", err, out.String())
	}
	if strings.TrimSpace(out.String()) != "" {
		t.Errorf("guard-handoff.sh on docs/architecture.md = %q, want silent (not the handoff file)", out.String())
	}
}
