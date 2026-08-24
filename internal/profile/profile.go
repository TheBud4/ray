// Package profile define o modelo de receita (profile) do ray: o conjunto
// curado de componentes, integrações e scaffold que uma stack recebe. Só
// carrega, valida e persiste receitas — nunca executa nada.
package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// Profile é uma receita, normalmente guardada em ~/.ray/profiles/<name>.yaml.
type Profile struct {
	Name         string       `yaml:"name"`
	Description  string       `yaml:"description"`
	Integrations Integrations `yaml:"integrations"`
	Components   []Component  `yaml:"components"`
	Scaffold     Scaffold     `yaml:"scaffold"`
	Create       []string     `yaml:"create"`
}

// Integrations liga/desliga as capacidades embutidas que o ray conecta num projeto.
type Integrations struct {
	Headroom  bool `yaml:"headroom"`
	CodeGraph bool `yaml:"code_graph"`
}

// Component é um pacote de conteúdo (skill, agent, comando) que o usuário
// mantém localmente em <ComponentsDir>/<Name> — nunca baixado. `ray init ai`
// copia o conteúdo de lá para <projeto>/<Dest>/<Name>; `ray update` recopia
// pela mesma política de "o usuário editou isto?" que os arquivos de
// scaffold (store.DecideOverwrite).
type Component struct {
	// Name identifica o componente: o nome da subpasta em <ComponentsDir> e
	// também o nome que ele ocupa dentro de Dest no projeto.
	Name string `yaml:"name"`
	// Dest é o diretório-contêiner relativo ao projeto (ex. ".claude/skills",
	// ".claude/agents") onde <ComponentsDir>/<Name> é copiado.
	Dest string `yaml:"dest"`
}

// Scaffold descreve arquivos que o ray escreve e settings mesclados no .claude.
type Scaffold struct {
	Files    []ScaffoldFile `yaml:"files,omitempty"`
	Settings map[string]any `yaml:"settings,omitempty"`
	// GitignoreStack são linhas específicas da stack acrescentadas ao bloco
	// base do .gitignore (scaffold.MergeGitignore); podem usar text/template
	// (ex. "/{{.ProjectName}}"), renderizadas com os mesmos dados do scaffold.
	GitignoreStack []string `yaml:"gitignore_stack,omitempty"`
}

// ScaffoldFile é um arquivo a criar; Template nomeia um template de origem opcional.
type ScaffoldFile struct {
	Path     string `yaml:"path"`
	Template string `yaml:"template,omitempty"`
}

// Validate reporta o primeiro problema estrutural em p, ou nil se p é usável.
func (p *Profile) Validate() error {
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("name is required")
	}
	for i, c := range p.Components {
		if err := c.validate(); err != nil {
			return fmt.Errorf("component %d: %w", i, err)
		}
	}
	for i, f := range p.Scaffold.Files {
		if strings.TrimSpace(f.Path) == "" {
			return fmt.Errorf("scaffold file %d: path is required", i)
		}
	}
	return nil
}

func (c Component) validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(c.Dest) == "" {
		return fmt.Errorf("dest is required")
	}
	return nil
}

// LoadByName lê e valida a receita chamada name em profilesDir. É o caminho
// para quem tem um nome — que é todo mundo, já que nome é o que o usuário
// digita. Existe para o erro falar de receita: o os.ReadFile fala de arquivo,
// e devolver o caminho cru a quem digitou `--profile web` troca o vocabulário
// no meio do caminho.
func LoadByName(profilesDir, name string) (*Profile, error) {
	path := filepath.Join(profilesDir, name+".yaml")
	if _, err := os.Stat(path); err != nil && os.IsNotExist(err) {
		return nil, fmt.Errorf("profile %q not found in %s", name, profilesDir)
	}
	return Load(path)
}

// Load lê e valida a receita em path.
func Load(path string) (*Profile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var p Profile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if err := p.Validate(); err != nil {
		return nil, fmt.Errorf("invalid profile %s: %w", path, err)
	}
	return &p, nil
}

// ProfileRecordPath é onde `ray init ai` grava o nome do perfil usado
// (initai.go, passo 12) — permite a `ray update` descobrir o perfil de um
// projeto sem exigir --profile.
func ProfileRecordPath(target string) string {
	return filepath.Join(target, ".claude", ".ray-profile")
}

// LoadForTarget resolve e carrega o perfil de target: overrideName, se
// não-vazio, ganha; senão lê o registro project-local
// (ProfileRecordPath(target), escrito por `ray init ai`).
func LoadForTarget(profilesDir, target, overrideName string) (*Profile, error) {
	name := overrideName
	if name == "" {
		data, err := os.ReadFile(ProfileRecordPath(target))
		if err != nil {
			// O erro do os.ReadFile já carrega o caminho: envolvê-lo repetia
			// o caminho inteiro duas vezes na mesma linha. E "não existe"
			// merece frase própria — é o caso comum, e a saída é uma só.
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("no profile recorded at %s (pass --profile to choose one)", ProfileRecordPath(target))
			}
			return nil, fmt.Errorf("reading the recorded profile: %w", err)
		}
		name = strings.TrimSpace(string(data))
	}
	return LoadByName(profilesDir, name)
}
