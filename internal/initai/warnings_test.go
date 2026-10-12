package initai

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/TheBud4/ray/internal/report"
	"github.com/TheBud4/ray/internal/runner"
)

// O motivo de um item mora no Detail da entrada; repeti-lo em Warnings faz a
// tela dizer a mesma coisa duas vezes.
func TestRunDoesNotRepeatAMissingComponentInWarnings(t *testing.T) {
	home := newHome(t)
	writeProfile(t, home.ProfilesDir, testProfile())

	sum := runEntries(t, home, t.TempDir(), &runner.FakeRunner{}, Options{})

	if len(sum.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none: the missing component is already an entry with its path", sum.Warnings)
	}
}

func TestRunDoesNotRepeatAKeptComponentInWarnings(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	runEntries(t, home, target, &runner.FakeRunner{}, Options{})
	skill := filepath.Join(target, ".claude", "skills", "s", "SKILL.md")
	if err := os.WriteFile(skill, []byte("# edited by the user"), 0o644); err != nil {
		t.Fatal(err)
	}

	sum := runEntries(t, home, target, &runner.FakeRunner{}, Options{})

	if len(sum.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none: the preserved component is already an entry with its reason", sum.Warnings)
	}
}

func TestRunDoesNotRepeatAFailedGlobalInWarnings(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())
	fr := &runner.FakeRunner{Results: map[string]runner.Result{
		"uv tool install graphifyy": {ExitCode: 2, Stderr: "error: No space left on device\n"},
	}}

	sum := runEntries(t, home, t.TempDir(), fr, Options{})

	if len(sum.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none: the failed global is already an entry with its reason", sum.Warnings)
	}
}

// Um arquivo de scaffold que já existia fica como está (Kept): o ray não o
// escreveu, então não o sugere no `git add` do rodapé.
func TestRunKeepsAnExistingScaffoldFileOutOfTheVersionedPaths(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	if err := os.WriteFile(filepath.Join(target, "CLAUDE.md"), []byte("# mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	sum := runEntries(t, home, target, &runner.FakeRunner{}, Options{})

	assertHasEntries(t, sum, report.Entry{Kind: report.File, Name: "CLAUDE.md", Outcome: report.Kept, Detail: "already there"})
	if slices.Contains(sum.VersionedPaths, "CLAUDE.md") {
		t.Errorf("VersionedPaths = %v, must not suggest a file the ray did not write", sum.VersionedPaths)
	}
	if !slices.Contains(sum.VersionedPaths, ".claude") {
		t.Errorf("VersionedPaths = %v, want .claude (created by this run)", sum.VersionedPaths)
	}
}
