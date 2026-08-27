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

func TestCommandForGOOS(t *testing.T) {
	cases := []struct {
		goos string
		want string
	}{
		{"linux", "xdg-open /some/path"},
		{"freebsd", "xdg-open /some/path"},
		{"darwin", "open /some/path"},
		{"windows", `rundll32 url.dll,FileProtocolHandler /some/path`},
	}
	for _, tc := range cases {
		t.Run(tc.goos, func(t *testing.T) {
			got := commandForGOOS(tc.goos, "/some/path").String()
			if got != tc.want {
				t.Errorf("commandForGOOS(%q) = %q, want %q", tc.goos, got, tc.want)
			}
		})
	}
}

func TestOpenPropagatesRunnerError(t *testing.T) {
	fr := &runner.FakeRunner{Err: errors.New("fake failure")}

	if err := Open(fr, "/some/path"); err == nil {
		t.Fatal("Open() = nil error, want the runner's error propagated")
	}
}
