package preflight

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/TheBud4/ray/internal/runner"
)

// ctxRunner guarda o context recebido. O FakeRunner descarta o dele, e o que
// se quer afirmar aqui é justamente o prazo que o Look impõe.
type ctxRunner struct {
	got context.Context
}

func (r *ctxRunner) Run(ctx context.Context, _ runner.Command) (runner.Result, error) {
	r.got = ctx
	return runner.Result{ExitCode: 0}, nil
}

func TestRunnerLookerDeadline(t *testing.T) {
	cr := &ctxRunner{}
	l := RunnerLooker{Runner: cr}

	l.Look("npx")

	deadline, ok := cr.got.Deadline()
	if !ok {
		t.Fatal("Look ran the command with a context that has no deadline")
	}
	// Medido depois da chamada: o que sobra do prazo nunca pode passar do
	// prazo inteiro, e a comparação não precisa de folga por isso.
	if left := time.Until(deadline); left <= 0 || left > lookTimeout {
		t.Errorf("deadline leaves %v, want something in (0, %v]", left, lookTimeout)
	}
}

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
