// Package vault cria e reporta o estado do vault de conhecimento da IA
// (~/.ray/vault), compatível com Obsidian. Só toca filesystem.
package vault

import (
	"os"
	"path/filepath"
	"strings"
)

const readme = `# Vault

Vault de conhecimento da IA, gerido pelo ray. Compatível com Obsidian.

- ` + "`inbox/`" + ` — notas capturadas, ainda não organizadas.
- ` + "`notes/`" + ` — notas processadas.
`

// Ensure garante o layout mínimo de vault em dir: inbox/, notes/, .obsidian/ e
// README.md. Idempotente: nunca sobrescreve o que já existir.
func Ensure(dir string) error {
	for _, sub := range []string{"inbox", "notes", ".obsidian"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return err
		}
	}
	readmePath := filepath.Join(dir, "README.md")
	if _, err := os.Stat(readmePath); err == nil {
		return nil // já existe: respeita
	}
	return os.WriteFile(readmePath, []byte(readme), 0o644)
}

// Status é o retrato do vault em dir, usado por `ray vault status`.
type Status struct {
	Path          string
	Exists        bool
	MarkdownCount int
}

// Stat lê o estado do vault em dir. dir inexistente não é erro: devolve
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
