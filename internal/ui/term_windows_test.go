//go:build windows

package ui

import (
	"os"
	"path/filepath"
	"testing"
)

// No runner de CI o stdout é um pipe, não um console: GetConsoleMode falha e o
// modo auto tem de decidir sem cor. O dispositivo NUL também não é um console.
func TestNotAConsoleIsNotAColorTerminalOnWindows(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "plain"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if IsTerminal(f) || ColorTerminal(f) {
		t.Error("a regular file is not a console")
	}
	nul, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer nul.Close()
	if IsTerminal(nul) || ColorTerminal(nul) {
		t.Error("NUL is a character device but not a console")
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if ColorTerminal(w) {
		t.Error("a pipe is not a console")
	}
	if ColorEnabled(Auto, func(string) string { return "" }, func() bool { return ColorTerminal(w) }) {
		t.Error("auto must decide without color when stdout is not a console")
	}
}
