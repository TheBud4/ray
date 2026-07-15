package runner

import (
	"context"
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
