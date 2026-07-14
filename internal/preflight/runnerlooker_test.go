package preflight

import (
	"errors"
	"testing"

	"github.com/TheBud4/ray/internal/runner"
)

func TestRunnerLookerMissingBinary(t *testing.T) {
	fr := &runner.FakeRunner{Err: errors.New("exec: \"npx\": executable file not found in $PATH")}
	l := RunnerLooker{Runner: fr}

	if l.Look("npx") {
		t.Error("Look(npx) = true, want false when the runner errors")
	}
}

func TestRunnerLookerGenericSuccess(t *testing.T) {
	fr := &runner.FakeRunner{Results: map[string]runner.Result{
		"npx --version": {ExitCode: 0, Stdout: "10.9.2\n"},
	}}
	l := RunnerLooker{Runner: fr}

	if !l.Look("npx") {
		t.Error("Look(npx) = false, want true on exit code 0")
	}
}

func TestRunnerLookerGenericNonZeroExit(t *testing.T) {
	fr := &runner.FakeRunner{Results: map[string]runner.Result{
		"npx --version": {ExitCode: 1},
	}}
	l := RunnerLooker{Runner: fr}

	if l.Look("npx") {
		t.Error("Look(npx) = true, want false on non-zero exit code")
	}
}

func TestRunnerLookerPythonVersion(t *testing.T) {
	cases := []struct {
		name   string
		stdout string
		want   bool
	}{
		{"too old", "Python 3.9.0\n", false},
		{"newer major/minor", "Python 3.11.4\n", true},
		{"exact floor", "Python 3.10.0\n", true},
		{"future major", "Python 4.0.0\n", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fr := &runner.FakeRunner{Results: map[string]runner.Result{
				"python3 --version": {ExitCode: 0, Stdout: tc.stdout},
			}}
			l := RunnerLooker{Runner: fr}
			if got := l.Look("python3.10+"); got != tc.want {
				t.Errorf("Look(python3.10+) with %q = %v, want %v", tc.stdout, got, tc.want)
			}
		})
	}
}

func TestRunnerLookerPythonMissing(t *testing.T) {
	fr := &runner.FakeRunner{Err: errors.New("exec: \"python3\": executable file not found in $PATH")}
	l := RunnerLooker{Runner: fr}

	if l.Look("python3.10+") {
		t.Error("Look(python3.10+) = true, want false when python3 is missing")
	}
}
