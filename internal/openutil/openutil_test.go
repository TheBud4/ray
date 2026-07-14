package openutil

import (
	"errors"
	"runtime"
	"testing"

	"github.com/TheBud4/ray/internal/runner"
)

func TestOpenUsesPlatformCommand(t *testing.T) {
	fr := &runner.FakeRunner{}

	if err := Open(fr, "/some/path"); err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	want := "xdg-open /some/path"
	if runtime.GOOS == "darwin" {
		want = "open /some/path"
	}
	if len(fr.Calls) != 1 || fr.Calls[0].String() != want {
		t.Fatalf("Calls = %v, want [%q]", fr.Calls, want)
	}
}

func TestOpenPropagatesRunnerError(t *testing.T) {
	fr := &runner.FakeRunner{Err: errors.New("fake failure")}

	if err := Open(fr, "/some/path"); err == nil {
		t.Fatal("Open() = nil error, want the runner's error propagated")
	}
}
