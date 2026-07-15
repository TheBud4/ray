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
	// Milestones é um overlay opcional de projeto-escola (design §9.3,
	// I6a): cada marco é um comando verificável, não prosa. O `ray` só
	// fornece a máquina (internal/learn) — a curadoria do currículo vive
	// na receita do usuário.
	Milestones []Milestone `yaml:"milestones,omitempty"`
}

// Milestone é um marco de projeto-escola: Verify é o comando (sem shell,
// splitado por espaço como os passos de internal/runfile) que precisa
// passar para o marco contar como cruzado.
type Milestone struct {
	Goal   string `yaml:"goal"`
	Verify string `yaml:"verify"`
}

// Integrations liga/desliga as capacidades embutidas que o ray conecta num projeto.
type Integrations struct {
	Headroom        bool `yaml:"headroom"`
	KnowledgeVault  bool `yaml:"knowledge_vault"`
	SecondBrain     bool `yaml:"second_brain"`
	ObsidianFormats bool `yaml:"obsidian_formats"`
	CodeGraph       bool `yaml:"code_graph"`
	UserDocsVault   bool `yaml:"user_docs_vault"`
}

// Constantes nomeadas: viram exatamente o valor esperado no YAML.
const (
	ViaSkills = "skills"
	ViaAitmpl = "aitmpl"
	ViaGit    = "git"
)

const (
	TypeAgent   = "agent"
	TypeCommand = "command"
	TypeMCP     = "mcp"
)

// Component é uma unidade instalável vinda de um ecossistema externo.
// via: "skills" usa Skill+Source; via: "aitmpl" usa Type+Ref; via: "git" usa
// Repo+Ref+Path (I2: aquisição direta de um repositório, pinada por ref).
type Component struct {
	Via    string `yaml:"via"`
	Skill  string `yaml:"skill,omitempty"`
	Source string `yaml:"source,omitempty"`
	Type   string `yaml:"type,omitempty"`
	Ref    string `yaml:"ref,omitempty"`
	Repo   string `yaml:"repo,omitempty"`
	Path   string `yaml:"path,omitempty"`
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
	for i, m := range p.Milestones {
		if err := m.validate(); err != nil {
			return fmt.Errorf("milestone %d: %w", i, err)
		}
	}
	return nil
}

func (m Milestone) validate() error {
	if strings.TrimSpace(m.Goal) == "" {
		return fmt.Errorf("goal is required")
	}
	if strings.TrimSpace(m.Verify) == "" {
		return fmt.Errorf("verify is required")
	}
	return nil
}

func (c Component) validate() error {
	switch c.Via {
	case ViaSkills:
		if c.Skill == "" || c.Source == "" {
			return fmt.Errorf("via skills requires both 'skill' and 'source'")
		}
	case ViaAitmpl:
		switch c.Type {
		case TypeAgent, TypeCommand, TypeMCP:
		default:
			return fmt.Errorf("via aitmpl requires type agent|command|mcp, got %q", c.Type)
		}
		if c.Ref == "" {
			return fmt.Errorf("via aitmpl requires 'ref'")
		}
	case ViaGit:
		if c.Repo == "" || c.Path == "" {
			return fmt.Errorf("via git requires 'repo' and 'path' (ref is optional, defaults to main)")
		}
	case "":
		return fmt.Errorf("'via' is required")
	default:
		return fmt.Errorf("unknown 'via' %q", c.Via)
	}
	return nil
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
// (initai.go, passo 12) — permite a comandos como `ray update`/`ray learn
// check` descobrirem o perfil de um projeto sem exigir --profile.
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
			return nil, fmt.Errorf("no profile recorded at %s and --profile not given: %w", ProfileRecordPath(target), err)
		}
		name = strings.TrimSpace(string(data))
	}
	return Load(filepath.Join(profilesDir, name+".yaml"))
}
