// Package vault valida e reporta o estado do cérebro do usuário — uma vault
// Obsidian que o usuário já mantém. Só toca filesystem.
//
// O ray não é dono deste diretório: ele não cria layout nem escreve README.
// Quem cria a vault é o usuário, no Obsidian; o ray só verifica que o caminho
// aponta para algo utilizável e o expõe ao agente por MCP.
package vault

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Verify checa que dir existe e é um diretório. Não cria nada e não impõe
// estrutura: qualquer pasta de markdown serve como cérebro.
func Verify(dir string) error {
	if dir == "" {
		return fmt.Errorf("brain path is empty; run `ray brain set <path>`")
	}
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("brain path does not exist: %s", dir)
		}
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("brain path is not a directory: %s", dir)
	}
	return nil
}

// Status é o retrato do cérebro em dir, usado por `ray brain status`.
type Status struct {
	Path          string
	Exists        bool
	MarkdownCount int
}

// Stat lê o estado do cérebro em dir. dir inexistente não é erro: devolve
// Status{Exists: false}.
func Stat(dir string) (Status, error) {
	st := Status{Path: dir}
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return st, nil
		}
		return Status{}, err
	}
	if !info.IsDir() {
		return st, nil
	}
	st.Exists = true

	count := 0
	err = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && strings.HasSuffix(d.Name(), ".md") {
			count++
		}
		return nil
	})
	if err != nil {
		return Status{}, err
	}
	st.MarkdownCount = count
	return st, nil
}
