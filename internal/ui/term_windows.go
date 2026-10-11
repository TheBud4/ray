//go:build windows

package ui

import (
	"os"
	"syscall"
)

// enableVirtualTerminalProcessing liga a interpretação de sequências de escape
// no console (ENABLE_VIRTUAL_TERMINAL_PROCESSING).
const enableVirtualTerminalProcessing = 0x0004

// IsTerminal diz se f é um console. O dispositivo NUL também é um dispositivo
// de caractere, então o teste certo no Windows é pedir o modo do console.
func IsTerminal(f *os.File) bool {
	var mode uint32
	return syscall.GetConsoleMode(syscall.Handle(f.Fd()), &mode) == nil
}

// ColorTerminal diz se f é um console em que a cor pode ser emitida: liga o
// modo de escape do console e devolve falso se não conseguir.
func ColorTerminal(f *os.File) bool {
	h := syscall.Handle(f.Fd())
	var mode uint32
	if syscall.GetConsoleMode(h, &mode) != nil {
		return false
	}
	if mode&enableVirtualTerminalProcessing != 0 {
		return true
	}
	setConsoleMode := syscall.NewLazyDLL("kernel32.dll").NewProc("SetConsoleMode")
	r, _, _ := setConsoleMode.Call(uintptr(h), uintptr(mode|enableVirtualTerminalProcessing))
	return r != 0
}
