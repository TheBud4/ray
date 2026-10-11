package ui

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestColorEnabledPrecedence(t *testing.T) {
	cases := []struct {
		name string
		mode Mode
		env  map[string]string
		tty  bool
		want bool
	}{
		{"always wins over NO_COLOR", Always, map[string]string{"NO_COLOR": "1"}, false, true},
		{"always wins over TERM=dumb", Always, map[string]string{"TERM": "dumb"}, false, true},
		{"never wins over a terminal", Never, nil, true, false},
		{"auto, terminal", Auto, nil, true, true},
		{"auto, not a terminal", Auto, nil, false, false},
		{"auto, NO_COLOR set", Auto, map[string]string{"NO_COLOR": "1"}, true, false},
		{"auto, NO_COLOR empty does not disable", Auto, map[string]string{"NO_COLOR": ""}, true, true},
		{"auto, TERM=dumb", Auto, map[string]string{"TERM": "dumb"}, true, false},
		{"auto, TERM unset does not disable", Auto, nil, true, true},
		{"auto, TERM=xterm-256color", Auto, map[string]string{"TERM": "xterm-256color"}, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			called := false
			got := ColorEnabled(c.mode, envOf(c.env), func() bool { called = true; return c.tty })
			if got != c.want {
				t.Errorf("ColorEnabled = %v, want %v", got, c.want)
			}
			if c.mode != Auto && called {
				t.Error("the terminal probe must not run when the flag already decided")
			}
		})
	}
}

func TestModeValueRejectsAnythingOutsideTheThreeValues(t *testing.T) {
	var m ModeValue
	if m.String() != "auto" {
		t.Errorf("zero value = %q, want auto", m.String())
	}
	for _, ok := range []string{"auto", "always", "never"} {
		if err := m.Set(ok); err != nil || m.String() != ok {
			t.Errorf("Set(%q) = %v, String() = %q", ok, err, m.String())
		}
	}
	m.Set("always")
	for _, bad := range []string{"", "yes", "ALWAYS", "true", "1"} {
		if err := m.Set(bad); err == nil {
			t.Errorf("Set(%q) must fail", bad)
		}
		if m.Mode != Always {
			t.Errorf("a rejected Set(%q) changed the mode", bad)
		}
	}
}

// Stream só embrulha o fluxo de verdade quando a cor está ligada, e um buffer
// de teste só com --color always.
func TestStream(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	other, _ := os.Create(filepath.Join(t.TempDir(), "other"))
	defer other.Close()
	var buf bytes.Buffer
	none := envOf(nil)

	// O arquivo comum não é terminal: em auto fica o próprio *os.File.
	if got := Stream(f, f, Auto, none); got != f {
		t.Errorf("Stream(file, auto) = %T, want the file itself", got)
	}
	if got := Stream(f, f, Never, none); got != f {
		t.Errorf("Stream(file, never) = %T, want the file itself", got)
	}
	// always embrulha o fluxo de verdade, e Raw devolve o arquivo.
	got := Stream(f, f, Always, none)
	if p, ok := got.(*Printer); !ok || !p.Color() || Raw(got) != f {
		t.Errorf("Stream(file, always) = %T, want a colored Printer over the file", got)
	}
	// Um buffer posto por SetOut fica como está, e só vira Printer com always.
	if got := Stream(&buf, f, Auto, none); got != &buf {
		t.Errorf("Stream(buffer, auto) = %T, want the buffer itself", got)
	}
	if got := Stream(&buf, f, Always, none); !got.(*Printer).Color() || Raw(got) != &buf {
		t.Errorf("Stream(buffer, always) = %T", got)
	}
	// Um arquivo que não é o fluxo de verdade é tratado como um buffer qualquer.
	if got := Stream(other, f, Auto, none); got != other {
		t.Errorf("Stream(other file, auto) = %T, want it untouched", got)
	}
	// Um Printer que já está lá não é embrulhado de novo.
	p := New(&buf, true)
	if got := Stream(p, f, Always, none); got != p {
		t.Error("Stream must not wrap a Printer again")
	}
}
