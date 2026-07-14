package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/runfile"
	"github.com/TheBud4/ray/internal/runner"
)

func testCommands() map[string]runfile.Resolved {
	return map[string]runfile.Resolved{
		"test": {Name: "test", Description: "run tests", Steps: []string{"go test ./..."}, BaseDir: "/proj", Source: runfile.SourceProject},
		"lint": {Name: "lint", Description: "lint everything", Steps: []string{"echo one", "echo two"}, BaseDir: "/home", Source: runfile.SourceGlobal},
	}
}

func TestRunRunCmdListsWithNoAliasOrFlag(t *testing.T) {
	var out bytes.Buffer
	if err := runRunCmd(testCommands(), "", nil, false, &runner.FakeRunner{}, false, &out); err != nil {
		t.Fatalf("runRunCmd() error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "test") || !strings.Contains(got, "lint") {
		t.Errorf("output = %q, want it to list both aliases", got)
	}
	if !strings.Contains(got, runfile.SourceProject) || !strings.Contains(got, runfile.SourceGlobal) {
		t.Errorf("output = %q, want it to show each alias's source", got)
	}
}

func TestRunRunCmdUnknownAliasErrors(t *testing.T) {
	err := runRunCmd(testCommands(), "nope", nil, false, &runner.FakeRunner{}, false, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runRunCmd() = nil error, want error for an unknown alias")
	}
	if !strings.Contains(err.Error(), "--list") {
		t.Errorf("error = %q, want it to point at `ray run --list`", err.Error())
	}
}

func TestRunRunCmdAbortsOnFirstFailure(t *testing.T) {
	fr := &runner.FakeRunner{Results: map[string]runner.Result{
		"echo one": {ExitCode: 1},
	}}
	err := runRunCmd(testCommands(), "lint", nil, false, fr, false, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runRunCmd() = nil error, want error when the first step fails")
	}
	if len(fr.Calls) != 1 {
		t.Fatalf("Calls = %v, want exactly 1 (aborted before the 2nd step)", fr.Calls)
	}
}

func TestRunRunCmdAppendsExtraArgsToLastStep(t *testing.T) {
	fr := &runner.FakeRunner{}
	err := runRunCmd(testCommands(), "lint", []string{"--foo"}, false, fr, false, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("runRunCmd() error = %v", err)
	}
	if len(fr.Calls) != 2 {
		t.Fatalf("Calls = %v, want 2 steps to run", fr.Calls)
	}
	if fr.Calls[0].String() != "echo one" {
		t.Errorf("first call = %q, want unchanged \"echo one\"", fr.Calls[0].String())
	}
	if fr.Calls[1].String() != "echo two --foo" {
		t.Errorf("last call = %q, want extra args appended", fr.Calls[1].String())
	}
}

func TestSplitAliasArgs(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		dashAt    int
		wantAlias string
		wantExtra []string
	}{
		{"no args", nil, -1, "", nil},
		{"alias only", []string{"test"}, -1, "test", nil},
		{"alias with dash", []string{"test", "-run", "TestX"}, 1, "test", []string{"-run", "TestX"}},
		{"dash with no alias", []string{"-v"}, 0, "", []string{"-v"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			alias, extra := splitAliasArgs(tc.args, tc.dashAt)
			if alias != tc.wantAlias {
				t.Errorf("alias = %q, want %q", alias, tc.wantAlias)
			}
			if len(extra) != len(tc.wantExtra) {
				t.Fatalf("extra = %v, want %v", extra, tc.wantExtra)
			}
			for i := range extra {
				if extra[i] != tc.wantExtra[i] {
					t.Errorf("extra[%d] = %q, want %q", i, extra[i], tc.wantExtra[i])
				}
			}
		})
	}
}
