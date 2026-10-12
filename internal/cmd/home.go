package cmd

import (
	"os"

	"github.com/TheBud4/ray/internal/ui"
)

// userHome é a pasta home que shortHome abrevia; os testes a trocam.
var userHome = func() string {
	h, _ := os.UserHomeDir()
	return h
}

// shortHome abrevia a pasta home em "~" para exibir um caminho.
func shortHome(s string) string { return ui.ShortenHome(s, userHome()) }
