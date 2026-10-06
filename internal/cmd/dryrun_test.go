package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// execRoot roda o ray inteiro (flags incluídas) com RAY_HOME isolado: é o único
// jeito de provar que o --dry-run chega ao comando, e não só à função.
func execRoot(t *testing.T, args ...string) (string, error) {
	t.Helper()
	t.Cleanup(func() { flagDryRun = false })
	var out bytes.Buffer
	root := newRootCmd()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func dryRunHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("RAY_HOME", home)
	t.Setenv("RAY_BRAIN", "")
	return home
}

func TestDryRunProfileRemoveKeepsTheProfile(t *testing.T) {
	home := dryRunHome(t)
	writeTestProfile(t, filepath.Join(home, "profiles"), newTestProfile(nil))
	file := filepath.Join(home, "profiles", "test.yaml")

	out, err := execRoot(t, "profile", "remove", "test", "--dry-run")
	if err != nil {
		t.Fatalf("Execute() error = %v\n%s", err, out)
	}
	if _, err := os.Stat(file); err != nil {
		t.Errorf("profile file gone after `profile remove --dry-run` (%v)", err)
	}
	if !strings.Contains(out, "+ remove") {
		t.Errorf("output = %q, want a `+ remove ...` line", out)
	}
}

func TestDryRunProfileAddCreatesNothing(t *testing.T) {
	home := dryRunHome(t)

	out, err := execRoot(t, "profile", "add", "mine", "--dry-run")
	if err != nil {
		t.Fatalf("Execute() error = %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(home, "profiles", "mine.yaml")); !os.IsNotExist(err) {
		t.Errorf("profile created by `profile add --dry-run` (stat err = %v)", err)
	}
	if !strings.Contains(out, "+ add") {
		t.Errorf("output = %q, want a `+ add ...` line", out)
	}
}

func TestDryRunProfileEditDoesNotOpenTheEditor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as $EDITOR")
	}
	home := dryRunHome(t)
	writeTestProfile(t, filepath.Join(home, "profiles"), newTestProfile(nil))
	marker := filepath.Join(t.TempDir(), "editor-ran")
	script := filepath.Join(t.TempDir(), "editor.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDITOR", script)

	out, err := execRoot(t, "profile", "edit", "test", "--dry-run")
	if err != nil {
		t.Fatalf("Execute() error = %v\n%s", err, out)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("the editor ran under --dry-run (stat err = %v)", err)
	}
	if !strings.Contains(out, "+ edit") {
		t.Errorf("output = %q, want a `+ edit ...` line", out)
	}
}

// `profile list` lê; sob --dry-run não pode semear ~/.ray/profiles.
func TestDryRunProfileListDoesNotSeedTheProfilesDir(t *testing.T) {
	home := dryRunHome(t)

	if out, err := execRoot(t, "profile", "list", "--dry-run"); err != nil {
		t.Fatalf("Execute() error = %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(home, "profiles")); !os.IsNotExist(err) {
		t.Errorf("profiles dir created by `profile list --dry-run` (stat err = %v)", err)
	}
}

func TestDryRunBrainSetDoesNotWriteTheConfig(t *testing.T) {
	home := dryRunHome(t)
	brain := t.TempDir()

	out, err := execRoot(t, "brain", "set", brain, "--dry-run")
	if err != nil {
		t.Fatalf("Execute() error = %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(home, "config.yaml")); !os.IsNotExist(err) {
		t.Errorf("config.yaml written by `brain set --dry-run` (stat err = %v)", err)
	}
	if !strings.Contains(out, "+ ") {
		t.Errorf("output = %q, want the planned change printed", out)
	}
}

func TestDryRunBrainOpenDoesNotLaunchTheApp(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("fakes xdg-open on PATH")
	}
	home := dryRunHome(t)
	brain := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("brain: "+brain+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := t.TempDir()
	marker := filepath.Join(t.TempDir(), "opened")
	if err := os.WriteFile(filepath.Join(bin, "xdg-open"), []byte("#!/bin/sh\ntouch "+marker+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	out, err := execRoot(t, "brain", "open", "--dry-run")
	if err != nil {
		t.Fatalf("Execute() error = %v\n%s", err, out)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Errorf("the app was launched under --dry-run (stat err = %v)", err)
	}
	if !strings.Contains(out, "xdg-open") {
		t.Errorf("output = %q, want the planned `xdg-open` command printed", out)
	}
}
