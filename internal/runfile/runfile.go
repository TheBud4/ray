// Package runfile resolve os aliases do `ray run`, mesclando o `ray.yaml` do
// projeto (achado subindo a árvore) com o `~/.ray/commands.yaml` global.
package runfile

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Fontes possíveis de um alias resolvido.
const (
	SourceGlobal  = "global"
	SourceProject = "project"
)

// Command é uma entrada do bloco `commands:` de um arquivo de runfile.
type Command struct {
	Description string   `yaml:"description"`
	Steps       []string `yaml:"steps"`
}

// File é o formato de `ray.yaml`/`commands.yaml`.
type File struct {
	Commands map[string]Command `yaml:"commands"`
}

// Resolved é um alias já resolvido, pronto pra rodar.
type Resolved struct {
	Name        string
	Description string
	Steps       []string
	BaseDir     string // dir do ray.yaml (projeto) ou workdir (global)
	Source      string // SourceGlobal | SourceProject
}

// Load mescla o global (globalPath) com o projeto (ray.yaml, achado subindo a
// árvore a partir de workdir); o projeto sobrescreve o global por nome.
// Arquivos ausentes não são erro — só significam "sem aliases daquela fonte".
func Load(workdir, globalPath string) (map[string]Resolved, error) {
	absWorkdir, err := filepath.Abs(workdir)
	if err != nil {
		return nil, err
	}

	out := map[string]Resolved{}

	gf, err := loadFileOrEmpty(globalPath)
	if err != nil {
		return nil, err
	}
	for name, c := range gf.Commands {
		out[name] = Resolved{Name: name, Description: c.Description, Steps: c.Steps, BaseDir: absWorkdir, Source: SourceGlobal}
	}

	if projectFile, projectDir := findProjectFile(absWorkdir); projectFile != "" {
		pf, err := loadFileOrEmpty(projectFile)
		if err != nil {
			return nil, err
		}
		for name, c := range pf.Commands {
			out[name] = Resolved{Name: name, Description: c.Description, Steps: c.Steps, BaseDir: projectDir, Source: SourceProject}
		}
	}

	return out, nil
}

func loadFileOrEmpty(path string) (*File, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &File{}, nil
		}
		return nil, err
	}
	var f File
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// findProjectFile sobe a árvore a partir de dir procurando um ray.yaml.
// Devolve ("", "") se não achar até a raiz.
func findProjectFile(dir string) (path, foundDir string) {
	for {
		candidate := filepath.Join(dir, "ray.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ""
		}
		dir = parent
	}
}
