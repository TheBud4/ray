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
