// Package raypaths resolve onde o ray guarda seu estado em disco, com raiz em
// ~/.ray (ou $RAY_HOME, se definido).
package raypaths

import (
	"os"
	"path/filepath"
	"strings"
)

// Home devolve $RAY_HOME se definido, senão <home-do-usuário>/.ray. O
// $RAY_HOME é normalizado: `~` inicial vira a home do usuário (o shell não o
// expande dentro de aspas nem num arquivo de env) e um caminho relativo vira
// absoluto, para o estado não mudar de pasta com o diretório corrente.
func Home() (string, error) {
	home, err := os.UserHomeDir()
	if h := os.Getenv("RAY_HOME"); h != "" {
		if h == "~" || strings.HasPrefix(h, "~/") || strings.HasPrefix(h, "~"+string(filepath.Separator)) {
			if err != nil {
				return "", err
			}
			h = filepath.Join(home, h[1:])
		}
		return filepath.Abs(h)
	}
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".ray"), nil
}

// ProfilesDir é <Home>/profiles, onde ficam os YAML de receita.
func ProfilesDir() (string, error) { return sub("profiles") }

// TemplatesDir é <Home>/templates, o overlay editável de templates de scaffold.
func TemplatesDir() (string, error) { return sub("templates") }

// ConfigPath é <Home>/config.yaml (rayconfig.Config).
func ConfigPath() (string, error) { return sub("config.yaml") }

// StatePath é <Home>/state.yaml (rayconfig.State).
func StatePath() (string, error) { return sub("state.yaml") }

// CommandsPath é <Home>/commands.yaml (aliases globais do `ray run`).
func CommandsPath() (string, error) { return sub("commands.yaml") }

// StoreDir é <Home>/store, o cache content-addressed usado por
// internal/store para rastrear linha-base pristina (scaffold e componentes).
func StoreDir() (string, error) { return sub("store") }

// ComponentsDir é <Home>/components, o overlay local de skills/agents que o
// usuário mantém à mão — o ray nunca baixa nada, só copia de lá pro projeto.
func ComponentsDir() (string, error) { return sub("components") }

func sub(name string) (string, error) {
	h, err := Home()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, name), nil
}
