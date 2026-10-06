// Package profile define o modelo de receita (profile) do ray: o conjunto
// curado de componentes, integrações e scaffold que uma stack recebe. Só
// carrega, valida e persiste receitas — nunca executa nada.
package profile

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"

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
	if err := checkSingleSegment(p.Name); err != nil {
		return fmt.Errorf("name %w", err)
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
		if err := checkRelativeBelowRoot(f.Path); err != nil {
			return fmt.Errorf("scaffold file %d: path %w", i, err)
		}
		if f.Template != "" {
			if err := checkRelativeBelowRoot(f.Template); err != nil {
				return fmt.Errorf("scaffold file %d: template %w", i, err)
			}
		}
	}
	return nil
}

func (c Component) validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if err := checkSingleSegment(c.Name); err != nil {
		return fmt.Errorf("name %w", err)
	}
	if strings.TrimSpace(c.Dest) == "" {
		return fmt.Errorf("dest is required")
	}
	if err := checkRelativeBelowRoot(c.Dest); err != nil {
		return fmt.Errorf("dest %w", err)
	}
	return nil
}

// checkSingleSegment recusa um nome que não seja um único elemento de caminho:
// o nome vira parte de um caminho em disco, então separador, "." , ".." ou
// caractere de controle (inclusive NUL) poderiam sair do diretório esperado.
// Não há lista branca: acento, espaço, ponto e hífen continuam válidos.
func checkSingleSegment(v string) error {
	if v == "." || v == ".." || strings.ContainsAny(v, `/\`) || strings.IndexFunc(v, unicode.IsControl) >= 0 {
		return fmt.Errorf(`must be a single path element (no separators, ".", ".." or control characters)`)
	}
	return nil
}

// checkRelativeBelowRoot recusa um caminho que não fique estritamente abaixo
// da raiz a que é relativo: absoluto, ou que, depois de limpo, é a própria
// raiz (".") ou sobe dela (".." ou "../..."). Vale o valor limpo, então
// "a/../b" é válido e "a/.." (que vira ".") não.
func checkRelativeBelowRoot(v string) error {
	if filepath.IsAbs(v) || strings.HasPrefix(v, "/") || strings.HasPrefix(v, `\`) {
		return fmt.Errorf("must be a relative path below its root, not absolute")
	}
	c := filepath.ToSlash(filepath.Clean(v))
	if c == "." || c == ".." || strings.HasPrefix(c, "../") {
		return fmt.Errorf(`must be a relative path below its root (not ".", ".." or escaping it)`)
	}
	return nil
}

// PathFor devolve o caminho do arquivo da receita name dentro de dir. É o
// único ponto que transforma um nome digitado em caminho de disco: recusa o
// nome que não seja um único elemento de caminho, para que nenhum comando
// leia, grave ou apague fora de dir.
func PathFor(dir, name string) (string, error) {
	if err := checkSingleSegment(name); err != nil {
		return "", fmt.Errorf("profile name %q %w", name, err)
	}
	return filepath.Join(dir, name+".yaml"), nil
}

// LoadByName lê e valida a receita chamada name em profilesDir. É o caminho
// para quem tem um nome — que é todo mundo, já que nome é o que o usuário
// digita. Existe para o erro falar de receita: o os.ReadFile fala de arquivo,
// e devolver o caminho cru a quem digitou `--profile web` troca o vocabulário
// no meio do caminho.
func LoadByName(profilesDir, name string) (*Profile, error) {
	path, err := PathFor(profilesDir, name)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err != nil && os.IsNotExist(err) {
		return nil, fmt.Errorf("profile %q not found in %s", name, profilesDir)
	}
	return Load(path)
}

// LoadByNameOrDefault é LoadByName para quem não pode gravar o diretório de
// receitas (dry-run): o arquivo em dir ganha, como sempre; se ele não existe e
// name é de um perfil de fábrica, devolve uma cópia validada dele — o que
// EnsureDir teria gravado. Sem nenhum dos dois, o mesmo erro de LoadByName.
func LoadByNameOrDefault(dir, name string) (*Profile, error) {
	path, err := PathFor(dir, name)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err == nil || !os.IsNotExist(err) {
		return LoadByName(dir, name)
	}
	for _, d := range Defaults() {
		if d.Name == name {
			p := d
			if err := p.Validate(); err != nil {
				return nil, err
			}
			return &p, nil
		}
	}
	return LoadByName(dir, name)
}

// Load lê e valida a receita em path. Decodificação estrita (RF-03): uma
// chave de topo desconhecida — "scaffhold" por "scaffold", por exemplo — vira
// erro em vez de silenciosamente virar uma seção vazia. Sem isso a receita
// "funcionava" e simplesmente não escrevia nenhum arquivo, sem nada indicando
// o motivo.
func Load(path string) (*Profile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)
	var p Profile
	if err := dec.Decode(&p); err != nil {
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

// maxRecordBytes limita o tamanho do registro de perfil: ele guarda só um
// nome, então um arquivo maior que isso não é um registro legítimo.
const maxRecordBytes = 4096

// readRecord lê o registro em path sem passar de maxRecordBytes; um arquivo
// maior vira erro em vez de ser carregado inteiro na memória.
func readRecord(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxRecordBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxRecordBytes {
		return nil, fmt.Errorf("%s is too large (limit %d bytes)", path, maxRecordBytes)
	}
	return data, nil
}

// LoadForTarget resolve e carrega o perfil de target: overrideName, se
// não-vazio, ganha; senão lê o registro project-local
// (ProfileRecordPath(target), escrito por `ray init ai`).
func LoadForTarget(profilesDir, target, overrideName string) (*Profile, error) {
	name := overrideName
	if name == "" {
		data, err := readRecord(ProfileRecordPath(target))
		if err != nil {
			// O erro do os.ReadFile já carrega o caminho: envolvê-lo repetia
			// o caminho inteiro duas vezes na mesma linha. E "não existe"
			// merece frase própria — é o caso comum, e a saída é uma só.
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("no profile recorded at %s (run `ray init ai` to set up the environment here, or pass --profile to choose a recipe)", ProfileRecordPath(target))
			}
			return nil, fmt.Errorf("reading the recorded profile: %w", err)
		}
		name = strings.TrimSpace(string(data))
	}
	return LoadByName(profilesDir, name)
}
