// Package raypaths resolve onde o ray guarda seu estado em disco, com raiz em
// ~/.ray (ou $RAY_HOME, se definido).
package raypaths

import (
	"os"
	"path/filepath"
)

// Home devolve $RAY_HOME se definido, senão <home-do-usuário>/.ray.
func Home() (string, error) {
	if h := os.Getenv("RAY_HOME"); h != "" {
		return h, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ray"), nil
}

// ProfilesDir é <Home>/profiles, onde ficam os YAML de receita.
func ProfilesDir() (string, error) { return sub("profiles") }

// TemplatesDir é <Home>/templates, o overlay editável de templates de scaffold.
func TemplatesDir() (string, error) { return sub("templates") }

// VaultDir é <Home>/vault, o vault de conhecimento da IA.
func VaultDir() (string, error) { return sub("vault") }

// ConfigPath é <Home>/config.yaml (rayconfig.Config).
func ConfigPath() (string, error) { return sub("config.yaml") }

// StatePath é <Home>/state.yaml (rayconfig.State).
func StatePath() (string, error) { return sub("state.yaml") }

// CommandsPath é <Home>/commands.yaml (aliases globais do `ray run`).
func CommandsPath() (string, error) { return sub("commands.yaml") }

// StoreDir é <Home>/store, o cache content-addressed de conteúdo de IA
// adquirido (internal/store).
func StoreDir() (string, error) { return sub("store") }

func sub(name string) (string, error) {
	h, err := Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, name), nil
}
