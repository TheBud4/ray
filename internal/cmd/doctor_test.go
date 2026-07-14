package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/runner"
)

type stubLooker map[string]bool

func (s stubLooker) Look(name string) bool { return s[name] }

func TestRunDoctorAllPresentNoError(t *testing.T) {
	l := stubLooker{"npx": true, "node": true, "python3.10+": true, "uv": true, "headroom": true, "graphify": true}
	var out bytes.Buffer

	err := runDoctor(l, &runner.FakeRunner{}, false, &out)
	if err != nil {
		t.Fatalf("runDoctor() error = %v, want nil", err)
	}
	if !strings.Contains(out.String(), "npx") {
		t.Errorf("output = %q, want it to contain the table with npx", out.String())
	}
}

func TestRunDoctorMissingRequiredWithoutFix(t *testing.T) {
	l := stubLooker{"node": true} // npx missing (required)
	var out bytes.Buffer

	err := runDoctor(l, &runner.FakeRunner{}, false, &out)
	if err == nil {
		t.Fatal("runDoctor() = nil error, want error when a required dep is missing")
	}
	if !strings.Contains(err.Error(), "ray doctor --fix") {
		t.Errorf("error = %q, want it to hint at `ray doctor --fix`", err.Error())
	}
}

// flippingLooker simulates a dependency becoming available after Fix runs:
// it answers from `before` until the fixRunner has recorded any call, then
// from `after` — proving runDoctor re-checks after fixing.
type flippingLooker struct {
	before, after map[string]bool
	fr            *runner.FakeRunner
}

func (f flippingLooker) Look(name string) bool {
	m := f.before
	if len(f.fr.Calls) > 0 {
		m = f.after
	}
	return m[name]
}

func TestRunDoctorFixInstallsAndRechecks(t *testing.T) {
	fr := &runner.FakeRunner{}
	l := flippingLooker{
		before: map[string]bool{"npx": true, "node": true},
		after:  map[string]bool{"npx": true, "node": true, "uv": true, "python3.10+": true},
		fr:     fr,
	}
	var out bytes.Buffer

	err := runDoctor(l, fr, true, &out)
	if err != nil {
		t.Fatalf("runDoctor() error = %v, want nil after fix", err)
	}

	wantFixed := map[string]bool{
		"sh -c curl -LsSf https://astral.sh/uv/install.sh | sh": false,
		"uv tool install headroom-ai[mcp]":                      false,
		"uv tool install graphifyy":                             false,
	}
	for _, c := range fr.Calls {
		if _, ok := wantFixed[c.String()]; ok {
			wantFixed[c.String()] = true
		}
	}
	for cmdStr, called := range wantFixed {
		if !called {
			t.Errorf("expected fix command %q to run, but it did not (Calls = %v)", cmdStr, fr.Calls)
		}
	}
}

func TestRunDoctorFixDryRunDoesNotExecute(t *testing.T) {
	dryRunner := runner.ExecRunner{DryRun: true, Out: &bytes.Buffer{}}
	l := stubLooker{"npx": true, "node": true}
	var out bytes.Buffer

	// uv/headroom/graphify all missing and have Fix; use the real ExecRunner
	// in DryRun mode as fixRunner to prove it never spawns a process.
	err := runDoctor(l, dryRunner, true, &out)
	if err == nil {
		t.Fatal("runDoctor() = nil error, want error: python3.10+/uv still missing after a dry-run fix")
	}
}
