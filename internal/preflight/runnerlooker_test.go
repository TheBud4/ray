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

// keyedErrRunner permite programar erro para um comando específico e
// resultado para outro — o FakeRunner só tem um Err global, insuficiente
// para simular "python3 ausente, python presente".
type keyedErrRunner struct {
	err map[string]error
	res map[string]runner.Result
}

func (r keyedErrRunner) Run(_ context.Context, c runner.Command) (runner.Result, error) {
	if err, ok := r.err[c.String()]; ok {
		return runner.Result{}, err
	}
	return r.res[c.String()], nil
}

// No Windows o instalador oficial expõe "python", não "python3". O
// runtime.GOOS de quem roda o teste é fixo (Linux), então o fallback é
// verificado direto na função interna parametrizada por goos.
func TestRunnerLookerPythonWindowsFallback(t *testing.T) {
	fr := keyedErrRunner{
		err: map[string]error{"python3 --version": errors.New("exec: \"python3\": executable file not found in $PATH")},
		res: map[string]runner.Result{"python --version": {ExitCode: 0, Stdout: "Python 3.11.4\n"}},
	}
	l := RunnerLooker{Runner: fr}

	if !l.lookGOOS("python3.10+", "windows") {
		t.Error("lookGOOS(python3.10+, windows) = false, want true — falls back to python when python3 is missing")
	}
}

func TestRunnerLookerPythonNoFallbackOutsideWindows(t *testing.T) {
	fr := keyedErrRunner{
		err: map[string]error{"python3 --version": errors.New("exec: \"python3\": executable file not found in $PATH")},
		res: map[string]runner.Result{"python --version": {ExitCode: 0, Stdout: "Python 3.11.4\n"}},
	}
	l := RunnerLooker{Runner: fr}

	if l.lookGOOS("python3.10+", "linux") {
		t.Error("lookGOOS(python3.10+, linux) = true, want false — no fallback to python outside windows")
	}
}
