package runner

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestFakeRecordCalls(t *testing.T) {
	f := &FakeRunner{}

	_, _ = f.Run(context.Background(), Command{
		Name: "npx",
		Args: []string{"x"}})

	if len(f.Calls) != 1 || f.Calls[0].Name != "npx" {
		t.Fatalf("Call Not Recorded: %+v", f.Calls)
	}
}

func TestExecRunnerEcho(t *testing.T) {

	r := &ExecRunner{}
	res, err := r.Run(context.Background(), Command{Name: "echo", Args: []string{"hi"}})

	if err != nil {
		t.Fatal(err)
	}

	if res.ExitCode != 0 || res.Stdout != "hi\n" {
		t.Fatalf("Unexpected: %+v", res)
	}
}

func TestExecRunnerPropagatesEnv(t *testing.T) {
	r := &ExecRunner{}
	res, err := r.Run(context.Background(), Command{
		Name: "sh",
		Args: []string{"-c", "echo $DO_NOT_TRACK"},
		Env:  map[string]string{"DO_NOT_TRACK": "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Stdout != "1\n" {
		t.Fatalf("Stdout = %q, want %q (Env not propagated to the subprocess)", res.Stdout, "1\n")
	}
}

func TestExecRunnerNilEnvInheritsProcessEnv(t *testing.T) {
	t.Setenv("RAY_TEST_INHERIT", "yes")
	r := &ExecRunner{}
	res, err := r.Run(context.Background(), Command{
		Name: "sh",
		Args: []string{"-c", "echo $RAY_TEST_INHERIT"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Stdout != "yes\n" {
		t.Fatalf("Stdout = %q, want %q (nil Env should still inherit the process env)", res.Stdout, "yes\n")
	}
}

// FailureReason é o motivo curto que o resumo mostra: vazio no sucesso, o erro
// quando o processo nem rodou, e o exit com a última linha do stderr quando saiu
// com código diferente de zero.
func TestFailureReason(t *testing.T) {
	long := strings.Repeat("x", 300)
	cases := []struct {
		name string
		res  Result
		err  error
		want string
	}{
		{"success", Result{}, nil, ""},
		{"process could not run", Result{}, errors.New("exec: \"uv\": not found"), `exec: "uv": not found`},
		{"exit without stderr", Result{ExitCode: 2}, nil, "exit 2"},
		{"exit with stderr", Result{ExitCode: 22, Stderr: "curl: (6) Could not resolve host\n"}, nil, "exit 22: curl: (6) Could not resolve host"},
		{"last non-empty stderr line wins", Result{ExitCode: 1, Stderr: "warning: noise\nreal error\n\n"}, nil, "exit 1: real error"},
		{"long line is cut", Result{ExitCode: 1, Stderr: long}, nil, "exit 1: " + long[:200] + "..."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := FailureReason(tc.res, tc.err); got != tc.want {
				t.Errorf("FailureReason() = %q, want %q", got, tc.want)
			}
		})
	}
}
