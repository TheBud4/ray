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

func TestCheckPassRecordsStateLogAndHead(t *testing.T) {
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

	logData, err := os.ReadFile(LogPath(target))
	if err != nil {
		t.Fatalf("stat log: %v", err)
	}
	if !strings.Contains(string(logData), "Skeleton compiles") {
		t.Errorf("log = %q, want it to mention the passed milestone", logData)
	}

	headData, err := os.ReadFile(HeadPath(target))
	if err != nil {
		t.Fatalf("stat head: %v", err)
	}
	if !strings.Contains(string(headData), "1/2") || !strings.Contains(string(headData), "Tests are green") {
		t.Errorf("head = %q, want it to show 1/2 passed and the next goal", headData)
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

	headData, err := os.ReadFile(HeadPath(target))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(headData), "All milestones complete") {
		t.Errorf("head = %q, want \"All milestones complete\"", headData)
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
	if got := LogPath(target); got != filepath.Join(JournalDir(target), "learning-journal-log.md") {
		t.Errorf("LogPath() = %q", got)
	}
	if got := PassedPath(target); got != filepath.Join(JournalDir(target), "milestones-passed.yaml") {
		t.Errorf("PassedPath() = %q", got)
	}
}
