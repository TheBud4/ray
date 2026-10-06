// Package openutil abre um caminho no aplicativo default do sistema
// (xdg-open no Linux, open no macOS, rundll32 no Windows).
package openutil

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/TheBud4/ray/internal/runner"
)

// Open abre path no app default, rodando o comando através de r — a mesma
// fronteira de processos usada em todo o ray (docs/architecture.md).
func Open(r runner.Runner, path string) error {
	c := commandForGOOS(runtime.GOOS, path)
	res, err := r.Run(context.Background(), c)
	if err != nil {
		return err
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("%s exited with code %d: %s", c, res.ExitCode, strings.TrimSpace(res.Stderr))
	}
	return nil
}

// commandForGOOS isola a escolha de comando do runtime.GOOS real da máquina,
// para que os três ramos (incluindo Windows) sejam testáveis sem rodar em
// cada SO.
func commandForGOOS(goos, path string) runner.Command {
	switch goos {
	case "darwin":
		return runner.Command{Name: "open", Args: []string{path}}
	case "windows":
		// rundll32 evita a fronteira de aspas do `cmd /c start` (que trata o
		// primeiro argumento entre aspas como título de janela) e não abre
		// uma janela de console visível.
		return runner.Command{Name: "rundll32", Args: []string{"url.dll,FileProtocolHandler", path}}
	default:
		return runner.Command{Name: "xdg-open", Args: []string{path}}
	}
}
