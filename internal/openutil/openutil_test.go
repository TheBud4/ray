package openutil

import (
	"errors"
	"runtime"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/runner"
)

func TestOpenUsesPlatformCommand(t *testing.T) {
	fr := &runner.FakeRunner{}

	if err := Open(fr, "/some/path"); err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	// O esperado é escrito por SO, não derivado de commandForGOOS: assim o
	// teste ainda pega uma troca de comando no SO em que roda.
	want := "xdg-open /some/path"
	switch runtime.GOOS {
	case "darwin":
		want = "open /some/path"
	case "windows":
		want = "rundll32 url.dll,FileProtocolHandler /some/path"
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

// O xdg-open sem opener instalado (ou sem sessão gráfica) sai ≠ 0 sem ser erro
// de execução: o Runner devolve o exit no Result e err nil. Ignorá-lo fazia o
// `ray brain open` imprimir sucesso sem ter aberto nada.
func TestOpenFailsWhenTheOpenerExitsNonZero(t *testing.T) {
	cmd := commandForGOOS(runtime.GOOS, "/some/path")
	fr := &runner.FakeRunner{Results: map[string]runner.Result{
		cmd.String(): {ExitCode: 3, Stderr: "no method available for opening"},
	}}

	err := Open(fr, "/some/path")
	if err == nil {
		t.Fatal("Open() = nil, want an error when the opener exits non-zero")
	}
	if !strings.Contains(err.Error(), "no method available") {
		t.Errorf("error = %q, want the opener's stderr in it", err)
	}
}
