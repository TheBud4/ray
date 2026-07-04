// Package runner is the only frontier from ray to external processes.
package runner

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
)

// Command represents a process to execute.
// The zero-value Command{} is invalid (Name is empty), but Args/Dir are optional.
type Command struct {
	Name string
	Args []string
	Dir  string // Working directory ("" = actual).
}

// String gives a legible form to logs and error messages.
func (c Command) String() string {
	return strings.TrimSpace(c.Name + " " + strings.Join(c.Args, " "))
}

// Result is what is left after running a Command.
type Result struct {
	Stdout, Stderr string
	ExitCode       int
}

// Runner is the contract: it knows how to execute a Command.
// Everything in ray depends on this interface, never on exec directly — that's what enables FakeRunner in tests.
type Runner interface {
	Run(ctx context.Context, c Command) (Result, error)
}

// ExecRunner runs the actual commands. If the DryRun flag is active, it only prints them.
type ExecRunner struct {
	DryRun bool
	Out    io.Writer
}

func (r ExecRunner) Run(ctx context.Context, c Command) (Result, error) {

	if r.DryRun {
		if r.Out != nil {
			fmt.Fprintln(r.Out, "+ "+c.String())
		}
		return Result{ExitCode: 0}, nil
	}

	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Dir = c.Dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	res := Result{Stdout: stdout.String(), Stderr: stderr.String()}

	var ee *exec.ExitError
	if errors.As(err, &ee) {
		res.ExitCode = ee.ExitCode()
		return res, nil
	}
	if err != nil {
		return res, err
	}

	return res, nil
}
