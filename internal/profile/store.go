package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// EnsureDir escreve todo perfil default ainda ausente em dir, nunca
// sobrescrevendo um arquivo existente.
func EnsureDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, p := range Defaults() {
		path := filepath.Join(dir, p.Name+".yaml")
		if _, err := os.Stat(path); err == nil {
			continue // já existe: respeita
		}
		data, err := yaml.Marshal(p)
		if err != nil {
			return err
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// Entry é o resumo leve usado por `profile list`.
type Entry struct {
	Name        string
	Description string
}

// List devolve um resumo ordenado de cada *.yaml em dir. Arquivos que não
// parseiam são ignorados — `profile show` reporta o erro daquele arquivo
// especificamente; list nunca falha por causa de um arquivo quebrado.
func List(dir string) ([]Entry, error) {
	des, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Entry
	for _, de := range des {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".yaml") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(dir, de.Name()))
		if err != nil {
			continue
		}
		var p Profile
		if yaml.Unmarshal(data, &p) != nil {
			continue
		}
		name := p.Name
		if name == "" {
			name = strings.TrimSuffix(de.Name(), ".yaml")
		}
		out = append(out, Entry{Name: name, Description: p.Description})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// Starter devolve um perfil mínimo válido, para `profile add <name>`.
func Starter(name string) *Profile {
	return &Profile{Name: name, Description: fmt.Sprintf("Custom %s profile", name)}
}

// WriteNew grava p em <dir>/<p.Name>.yaml, falhando se já existir.
func WriteNew(dir string, p *Profile) error {
	if err := p.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, p.Name+".yaml")
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("profile %q already exists", p.Name)
	}
	data, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// Remove apaga <dir>/<name>.yaml.
func Remove(dir, name string) error {
	return os.Remove(filepath.Join(dir, name+".yaml"))
}
