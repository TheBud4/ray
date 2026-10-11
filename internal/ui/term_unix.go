//go:build !windows

package ui

import "os"

// IsTerminal diz se f é um dispositivo de caractere. Falso positivo só para
// /dev/null, onde cor é inofensiva.
func IsTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// ColorTerminal diz se f é um terminal em que a cor pode ser emitida. Fora do
// Windows não há nada a ligar.
func ColorTerminal(f *os.File) bool { return IsTerminal(f) }
