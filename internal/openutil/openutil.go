// Package openutil abre um caminho no aplicativo default do sistema
// (xdg-open no Linux, open no macOS).
package openutil

import (
	"context"
	"runtime"

	"github.com/TheBud4/ray/internal/runner"
)

// Open abre path no app default, rodando o comando através de r — a mesma
// fronteira de processos usada em todo o ray (§3 do build guide).
func Open(r runner.Runner, path string) error {
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	_, err := r.Run(context.Background(), runner.Command{Name: name, Args: []string{path}})
	return err
}
