//go:build !windows

package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsTerminalOnUnix(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "plain"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if IsTerminal(f) {
		t.Error("a regular file is not a terminal")
	}
	null, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	// /dev/null é um dispositivo de caractere: o falso positivo conhecido, em
	// que cor é inofensiva. O teste o documenta.
	if !IsTerminal(null) {
		t.Error("/dev/null is a character device; the documented false positive")
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if IsTerminal(w) {
		t.Error("a pipe is not a terminal")
	}
}
