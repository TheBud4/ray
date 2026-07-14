// Package rayconfig lê e grava o Config (~/.ray/config.yaml) e o State
// (~/.ray/state.yaml) do ray.
package rayconfig

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Config é a configuração persistente do usuário.
type Config struct {
	UserDocsVault string `yaml:"user_docs_vault"`
}

// Load lê o Config em path. Arquivo ausente devolve &Config{} (não é erro —
// primeira execução do ray).
func Load(path string) (*Config, error) {
	var c Config
	if err := loadYAML(path, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// Save grava c em path, criando o diretório pai se preciso.
func (c *Config) Save(path string) error {
	return saveYAML(path, c)
}

// SetUserDocsVault seta o campo e persiste imediatamente em path.
func (c *Config) SetUserDocsVault(path, docsPath string) error {
	c.UserDocsVault = docsPath
	return c.Save(path)
}

// UserDocsVaultPath devolve o vault central efetivo: RAY_DOCS_VAULT tem
// prioridade sobre o valor gravado em config.yaml.
func (c *Config) UserDocsVaultPath() string {
	if v := os.Getenv("RAY_DOCS_VAULT"); v != "" {
		return v
	}
	return c.UserDocsVault
}

// State rastreia globais já instalados (install-once).
type State struct {
	InstalledGlobals []string `yaml:"installed_globals"`
}

// LoadState lê o State em path. Arquivo ausente devolve &State{}.
func LoadState(path string) (*State, error) {
	var s State
	if err := loadYAML(path, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Save grava s em path, criando o diretório pai se preciso.
func (s *State) Save(path string) error {
	return saveYAML(path, s)
}

// HasGlobal reporta se key já está registrado como instalado.
func (s *State) HasGlobal(key string) bool {
	for _, k := range s.InstalledGlobals {
		if k == key {
			return true
		}
	}
	return false
}

// AddGlobal registra key como instalado; idempotente (não duplica).
func (s *State) AddGlobal(key string) {
	if s.HasGlobal(key) {
		return
	}
	s.InstalledGlobals = append(s.InstalledGlobals, key)
}

func loadYAML(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return yaml.Unmarshal(data, v)
}

func saveYAML(path string, v any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
