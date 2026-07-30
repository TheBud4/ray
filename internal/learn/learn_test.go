package learn

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/runner"
)

func milestones() []profile.Milestone {
	return []profile.Milestone{
		{Goal: "Skeleton compiles", Verify: "true"},
		{Goal: "Tests are green", Verify: "true"},
	}
}

// ---- Current (pure) ------------------------------------------------------

func TestCurrentNonePassed(t *testing.T) {
	m, ok := Current(milestones(), nil)
	if !ok || m.Goal != "Skeleton compiles" {
		t.Fatalf("Current() = (%v, %v), want (Skeleton compiles, true)", m, ok)
	}
}

func TestCurrentSomePassed(t *testing.T) {
	m, ok := Current(milestones(), []string{"Skeleton compiles"})
	if !ok || m.Goal != "Tests are green" {
		t.Fatalf("Current() = (%v, %v), want (Tests are green, true)", m, ok)
	}
}

func TestCurrentAllPassed(t *testing.T) {
	_, ok := Current(milestones(), []string{"Skeleton compiles", "Tests are green"})
	if ok {
		t.Fatal("Current() ok = true, want false when all milestones passed")
	}
}

func TestCurrentNoMilestones(t *testing.T) {
	_, ok := Current(nil, nil)
	if ok {
		t.Fatal("Current() ok = true, want false with no milestones defined")
	}
}

// ---- Check ----------------------------------------------------------------

func TestCheckNothingToCheckWithoutMilestones(t *testing.T) {
	target := t.TempDir()
	_, ok, err := Check(&runner.FakeRunner{}, target, nil)
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if ok {
		t.Fatal("Check() ok = true, want false with no milestones")
	}
}

func TestCheckPassRecordsStateAndProgress(t *testing.T) {
	target := t.TempDir()
	fr := &runner.FakeRunner{Results: map[string]runner.Result{
		"true": {ExitCode: 0},
	}}

	res, ok, err := Check(fr, target, milestones())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !ok {
		t.Fatal("Check() ok = false, want true")
	}
	if !res.Passed || res.Milestone.Goal != "Skeleton compiles" {
		t.Fatalf("Check() result = %+v, want Passed=true for Skeleton compiles", res)
	}

	passed, err := loadPassed(target)
	if err != nil {
		t.Fatal(err)
	}
	if len(passed) != 1 || passed[0] != "Skeleton compiles" {
		t.Errorf("passed state = %v, want [Skeleton compiles]", passed)
	}

	progressData, err := os.ReadFile(MilestonesProgressPath(target))
	if err != nil {
		t.Fatalf("stat progress: %v", err)
	}
	if !strings.Contains(string(progressData), "1/2") || !strings.Contains(string(progressData), "Tests are green") {
		t.Errorf("progress = %q, want it to show 1/2 passed and the next goal", progressData)
	}
}

func TestCheckNeverTouchesTheJournal(t *testing.T) {
	target := t.TempDir()
	if err := os.MkdirAll(JournalDir(target), 0o755); err != nil {
		t.Fatal(err)
	}

	// O diário é da IA: um contrato escrito por ela precisa sobreviver a
	// quantas passagens de marco vierem.
	journal := "## Combinado\n- Degrau inicial: 2\n"
	if err := os.WriteFile(HeadPath(target), []byte(journal), 0o644); err != nil {
		t.Fatal(err)
	}

	_, ok, err := Check(&runner.FakeRunner{}, target, milestones())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !ok {
		t.Fatal("Check() ok = false, want true com marcos definidos")
	}

	got, err := os.ReadFile(HeadPath(target))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != journal {
		t.Errorf("Check() reescreveu o diário.\n got: %q\nwant: %q", got, journal)
	}

	progress, err := os.ReadFile(MilestonesProgressPath(target))
	if err != nil {
		t.Fatalf("progresso não foi escrito: %v", err)
	}
	if !strings.Contains(string(progress), "1/2") {
		t.Errorf("progresso = %q, want conter \"1/2\"", progress)
	}
}

func TestCheckFailTouchesNothing(t *testing.T) {
	target := t.TempDir()
	fr := &runner.FakeRunner{Results: map[string]runner.Result{
		"true": {ExitCode: 1, Stderr: "boom"},
	}}

	res, ok, err := Check(fr, target, milestones())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if !ok {
		t.Fatal("Check() ok = false, want true (there was something to check)")
	}
	if res.Passed {
		t.Fatal("Check() Passed = true, want false")
	}
	if !strings.Contains(res.Output, "boom") {
		t.Errorf("Output = %q, want it to include the failure output", res.Output)
	}

	if _, err := os.Stat(JournalDir(target)); !os.IsNotExist(err) {
		t.Error("journal dir should not exist after a failed check — no blow-by-blow in the diary")
	}
}

func TestCheckNothingLeftAfterAllPassed(t *testing.T) {
	target := t.TempDir()
	fr := &runner.FakeRunner{Results: map[string]runner.Result{"true": {ExitCode: 0}}}

	if _, _, err := Check(fr, target, milestones()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := Check(fr, target, milestones()); err != nil {
		t.Fatal(err)
	}

	_, ok, err := Check(fr, target, milestones())
	if err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if ok {
		t.Fatal("Check() ok = true, want false once all milestones have passed")
	}

	progressData, err := os.ReadFile(MilestonesProgressPath(target))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(progressData), "All milestones complete") {
		t.Errorf("progress = %q, want \"All milestones complete\"", progressData)
	}
}

func TestCheckRunnerErrorSurfacesWithoutStateChange(t *testing.T) {
	target := t.TempDir()
	res, ok, err := Check(errRunner{}, target, milestones())
	if err == nil {
		t.Fatal("Check() error = nil, want the runner's error surfaced")
	}
	if !ok {
		t.Error("Check() ok = false, want true: there was a milestone to check, it just errored")
	}
	_ = res
	if _, statErr := os.Stat(JournalDir(target)); !os.IsNotExist(statErr) {
		t.Error("journal dir should not exist after a runner error")
	}
}

type errRunner struct{}

func (errRunner) Run(context.Context, runner.Command) (runner.Result, error) {
	return runner.Result{}, os.ErrPermission
}

func TestPathHelpers(t *testing.T) {
	target := "/proj"
	if got := JournalDir(target); got != filepath.Join(target, ".claude", ".local") {
		t.Errorf("JournalDir() = %q", got)
	}
	if got := HeadPath(target); got != filepath.Join(JournalDir(target), "learning-journal.md") {
		t.Errorf("HeadPath() = %q", got)
	}
	if got := MilestonesProgressPath(target); got != filepath.Join(JournalDir(target), "milestones-progress.md") {
		t.Errorf("MilestonesProgressPath() = %q", got)
	}
	if got := PassedPath(target); got != filepath.Join(JournalDir(target), "milestones-passed.yaml") {
		t.Errorf("PassedPath() = %q", got)
	}
}
