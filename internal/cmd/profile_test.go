package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/profile"
)

func TestRunProfileListIncludesDefaultsAndExtra(t *testing.T) {
	dir := t.TempDir()
	if err := runProfileAdd(dir, "custom"); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runProfileList(dir, &out); err != nil {
		t.Fatalf("runProfileList() error = %v", err)
	}

	got := out.String()
	for _, name := range []string{"go", "web", "flutter", "custom"} {
		if !strings.Contains(got, name) {
			t.Errorf("output = %q, want it to contain %q", got, name)
		}
	}
}

func TestRunProfileShowPrintsComponentsAndServers(t *testing.T) {
	dir := t.TempDir()
	p := &profile.Profile{
		Name:         "test",
		Description:  "a test profile",
		Integrations: profile.Integrations{Brain: true},
		Components:   []profile.Component{{Via: profile.ViaSkills, Skill: "s", Source: "o/r"}},
	}
	if err := profile.WriteNew(dir, p); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runProfileShow(dir, "test", "/home/u/www/MegaBrain", &out); err != nil {
		t.Fatalf("runProfileShow() error = %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "npx skills add o/r --skill s -a claude-code -y --copy") {
		t.Errorf("output = %q, want the component command (I2: preview forces --copy)", got)
	}
	if !strings.Contains(got, "brain") {
		t.Errorf("output = %q, want the brain MCP server", got)
	}
}

// O graphify é o servidor sem args da receita, e era ele que saía como
// "graphify: graphify-mcp " — um espaço pendurado no fim da linha, porque o
// formato juntava os args mesmo quando não havia nenhum.
func TestRunProfileShowLeavesNoTrailingSpaceOnAServerWithoutArgs(t *testing.T) {
	dir := t.TempDir()
	p := &profile.Profile{
		Name:         "test",
		Description:  "a test profile",
		Integrations: profile.Integrations{CodeGraph: true},
	}
	if err := profile.WriteNew(dir, p); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runProfileShow(dir, "test", "", &out); err != nil {
		t.Fatalf("runProfileShow() error = %v", err)
	}

	for _, line := range strings.Split(out.String(), "\n") {
		if strings.TrimRight(line, " ") != line {
			t.Errorf("line %q has trailing whitespace", line)
		}
	}
}

func TestRunProfileAddCreatesAndRejectsDuplicate(t *testing.T) {
	dir := t.TempDir()

	if err := runProfileAdd(dir, "custom"); err != nil {
		t.Fatalf("runProfileAdd() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom.yaml")); err != nil {
		t.Fatalf("stat custom.yaml: %v", err)
	}

	if err := runProfileAdd(dir, "custom"); err == nil {
		t.Fatal("runProfileAdd() = nil error, want error on duplicate name")
	}
}

func TestRunProfileEditRequiresEditorEnv(t *testing.T) {
	t.Setenv("EDITOR", "")

	err := runProfileEdit(t.TempDir(), "go", func(editor, path string) error {
		t.Fatal("spawn should not be called when $EDITOR is unset")
		return nil
	})
	if err == nil {
		t.Fatal("runProfileEdit() = nil error, want error when $EDITOR is unset")
	}
}

func TestRunProfileEditSpawnsWithEditorAndPath(t *testing.T) {
	t.Setenv("EDITOR", "nano")
	dir := t.TempDir()

	var gotEditor, gotPath string
	err := runProfileEdit(dir, "go", func(editor, path string) error {
		gotEditor, gotPath = editor, path
		return nil
	})
	if err != nil {
		t.Fatalf("runProfileEdit() error = %v", err)
	}
	if gotEditor != "nano" {
		t.Errorf("editor = %q, want nano", gotEditor)
	}
	if gotPath != filepath.Join(dir, "go.yaml") {
		t.Errorf("path = %q, want %q", gotPath, filepath.Join(dir, "go.yaml"))
	}
}

func TestRunProfileRemoveDeletesAndErrorsOnMissing(t *testing.T) {
	dir := t.TempDir()
	if err := runProfileAdd(dir, "custom"); err != nil {
		t.Fatal(err)
	}

	if err := runProfileRemove(dir, "custom"); err != nil {
		t.Fatalf("runProfileRemove() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "custom.yaml")); !os.IsNotExist(err) {
		t.Fatalf("custom.yaml should be gone, stat err = %v", err)
	}

	if err := runProfileRemove(dir, "does-not-exist"); err == nil {
		t.Fatal("runProfileRemove() = nil error, want error removing a missing profile")
	}
}
