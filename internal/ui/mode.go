package ui

import (
	"fmt"
	"io"
	"os"
)

// Mode é o valor de --color.
type Mode int

const (
	Auto Mode = iota
	Always
	Never
)

// ParseMode lê auto, always ou never.
func ParseMode(s string) (Mode, error) {
	switch s {
	case "auto":
		return Auto, nil
	case "always":
		return Always, nil
	case "never":
		return Never, nil
	}
	return Auto, fmt.Errorf("invalid color mode %q: use auto, always or never", s)
}

// ModeValue implementa pflag.Value para a flag --color. Valor fora de
// auto|always|never é recusado já no parse.
type ModeValue struct{ Mode Mode }

func (m *ModeValue) String() string {
	switch m.Mode {
	case Always:
		return "always"
	case Never:
		return "never"
	}
	return "auto"
}

func (m *ModeValue) Set(s string) error {
	v, err := ParseMode(s)
	if err != nil {
		return err
	}
	m.Mode = v
	return nil
}

func (m *ModeValue) Type() string { return "auto|always|never" }

// ColorEnabled decide se um fluxo tem cor, por prioridade: --color
// always|never; NO_COLOR definido e não vazio; TERM=dumb; e, em auto, se a
// saída é um terminal interativo. TERM ausente não desliga a cor (no Windows
// ele normalmente não existe). isTerminal só é chamada quando a decisão chega
// nela.
func ColorEnabled(mode Mode, getenv func(string) string, isTerminal func() bool) bool {
	switch mode {
	case Always:
		return true
	case Never:
		return false
	}
	if getenv("NO_COLOR") != "" {
		return false
	}
	if getenv("TERM") == "dumb" {
		return false
	}
	return isTerminal()
}

// Stream devolve w pronto para a raiz do cobra. f é o fluxo de verdade
// (os.Stdout ou os.Stderr). Se w já é um Printer, fica como está. Se w é o fluxo
// de verdade, vira um Printer colorido só quando a cor está ligada; sem cor,
// continua sendo o próprio *os.File. Se w é outra coisa (um buffer de teste),
// só vira Printer com --color always.
func Stream(w io.Writer, f *os.File, mode Mode, getenv func(string) string) io.Writer {
	if _, ok := w.(*Printer); ok {
		return w
	}
	if file, ok := w.(*os.File); ok && file == f {
		if ColorEnabled(mode, getenv, func() bool { return ColorTerminal(f) }) {
			return New(w, true)
		}
		return w
	}
	if mode == Always {
		return New(w, true)
	}
	return w
}
