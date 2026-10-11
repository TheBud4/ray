// Package catalog é o núcleo de leitura e edição do overlay de templates: o que
// cada template é (padrão, seu, defasado), o que o `init ai` fará com ele e se
// ele ainda renderiza. Não fala com o terminal nem com processo; quem o usa
// injeta a linha-base.
package catalog

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/TheBud4/ray/internal/fsutil"
	"github.com/TheBud4/ray/internal/scaffold"
	"github.com/TheBud4/ray/internal/store"
)

// State é o que o `init ai` faz com um template do overlay.
type State string

const (
	Current  State = "current"  // overlay == embutido
	Outdated State = "outdated" // o binário evoluiu e o usuário não tocou
	Edited   State = "edited"   // o usuário alterou: o init ai preserva
	New      State = "new"      // existe no embutido, ainda não no overlay
	Custom   State = "custom"   // existe no overlay, não no embutido
)

// Entry é um template e o seu estado. NoBaseline marca um Edited sem linha-base
// gravada (não há como saber se foi o usuário).
type Entry struct {
	Name       string
	State      State
	NoBaseline bool
}

// ErrUnknown é devolvido para um nome que List não conhece.
var ErrUnknown = errors.New("unknown template")

// Templates é o overlay em Dir e a linha-base dele, injetada.
type Templates struct {
	Dir         string
	Pristine    func(rel string) (string, bool)
	SetPristine func(rel, hash string) error
}

// List devolve os templates embutidos, com o estado que EnsureTemplates daria,
// e os custom do overlay, em ordem de nome. Só lê.
func (t Templates) List() ([]Entry, error) {
	synced, err := scaffold.EnsureTemplates(t.Dir, scaffold.EnsureOptions{DryRun: true, Pristine: t.Pristine})
	if err != nil {
		return nil, err
	}
	var out []Entry
	embedded := make(map[string]bool, len(synced))
	for _, s := range synced {
		embedded[s.Rel] = true
		e := Entry{Name: s.Rel}
		switch s.Action {
		case scaffold.TemplateCreated:
			e.State = New
		case scaffold.TemplateRefreshed:
			e.State = Outdated
		case scaffold.TemplateCurrent:
			e.State = Current
		case scaffold.TemplateKept:
			e.State = Edited
			_, has := t.Pristine(s.Rel)
			e.NoBaseline = !has
		}
		out = append(out, e)
	}
	customs, err := t.customs(embedded)
	if err != nil {
		return nil, err
	}
	out = append(out, customs...)
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// customs lista os *.tmpl do overlay fora do conjunto embutido, sem segmento
// oculto. A varredura usa os.DirFS, que segue a raiz se ela for um symlink.
func (t Templates) customs(embedded map[string]bool) ([]Entry, error) {
	var out []Entry
	err := fs.WalkDir(os.DirFS(t.Dir), ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return fs.SkipAll
			}
			return err
		}
		if d.IsDir() {
			if p != "." && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") || !strings.HasSuffix(p, ".tmpl") || embedded[p] {
			return nil
		}
		out = append(out, Entry{Name: p, State: Custom})
		return nil
	})
	return out, err
}

// Lookup devolve a entrada de name. O nome tem de ser exatamente um dos que List
// devolve; no Windows, "\" vira "/" antes de comparar.
func (t Templates) Lookup(name string) (Entry, error) {
	if filepath.Separator == '\\' {
		name = filepath.ToSlash(name)
	}
	entries, err := t.List()
	if err != nil {
		return Entry{}, err
	}
	for _, e := range entries {
		if e.Name == name {
			return e, nil
		}
	}
	return Entry{}, fmt.Errorf("%w %q (see `ray template list`)", ErrUnknown, name)
}

// Path devolve o caminho do arquivo de name no overlay, mesmo que ainda não exista.
func (t Templates) Path(name string) (string, error) {
	e, err := t.Lookup(name)
	if err != nil {
		return "", err
	}
	return filepath.Join(t.Dir, filepath.FromSlash(e.Name)), nil
}

// embeddedBytes lê o template embutido de name.
func embeddedBytes(name string) ([]byte, error) {
	return fs.ReadFile(scaffold.EmbeddedTemplates(), name)
}

// Embedded devolve o conteúdo do template embutido de name, para o diff.
func (t Templates) Embedded(name string) ([]byte, error) {
	e, err := t.Lookup(name)
	if err != nil {
		return nil, err
	}
	if e.State == Custom {
		return nil, fmt.Errorf("%s: custom template, there is no embedded one", e.Name)
	}
	return embeddedBytes(e.Name)
}

// Read devolve o conteúdo que o próximo `init ai` usaria: o embutido para New e
// Outdated (o init ai sincroniza antes de renderizar), o do overlay nos demais.
func (t Templates) Read(name string) ([]byte, error) {
	e, err := t.Lookup(name)
	if err != nil {
		return nil, err
	}
	if e.State == New || e.State == Outdated {
		return embeddedBytes(e.Name)
	}
	return os.ReadFile(filepath.Join(t.Dir, filepath.FromSlash(e.Name)))
}

// Overlay devolve o arquivo do overlay de name; ausente (New) vira nil sem erro.
func (t Templates) Overlay(name string) ([]byte, error) {
	e, err := t.Lookup(name)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(filepath.Join(t.Dir, filepath.FromSlash(e.Name)))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return b, err
}

// writeDefault grava o embutido de name no overlay (0644, pai criado) e a
// linha-base. Falha ao gravar a linha-base não é erro: devolve-a em baselineErr,
// porque sem ela o arquivo é Current, inofensivo. Alvo que é symlink é recusado.
func (t Templates) writeDefault(name string) (baselineErr, err error) {
	dest := filepath.Join(t.Dir, filepath.FromSlash(name))
	if err := t.refuseSymlink(name); err != nil {
		return nil, err
	}
	fresh, err := embeddedBytes(name)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return nil, err
	}
	if err := fsutil.WriteFileAtomic(dest, fresh, 0o644); err != nil {
		return nil, err
	}
	return t.SetPristine(name, store.HashBytes(fresh)), nil
}

// refuseSymlink recusa um alvo que é symlink: substituí-lo desfaria o link de um
// gerenciador de dotfiles, e o editor escreveria através dele.
func (t Templates) refuseSymlink(name string) error {
	dest := filepath.Join(t.Dir, filepath.FromSlash(name))
	if fi, err := os.Lstat(dest); err == nil && fi.Mode()&os.ModeSymlink != 0 {
		target, _ := os.Readlink(dest)
		return fmt.Errorf("%s is a symlink to %s; change the file it points to or remove the link", name, target)
	}
	return nil
}

// Prepare deixa name pronto para edição, como o `init ai` o deixaria: New e
// Outdated recebem o embutido e a linha-base; Current com linha-base ausente ou
// diferente a recebe; Edited e Custom não mudam. wrote diz se o arquivo foi gravado.
func (t Templates) Prepare(name string) (wrote bool, baselineErr error, err error) {
	e, err := t.Lookup(name)
	if err != nil {
		return false, nil, err
	}
	switch e.State {
	case New, Outdated:
		baselineErr, err = t.writeDefault(e.Name)
		return err == nil, baselineErr, err
	case Current:
		fresh, err := embeddedBytes(e.Name)
		if err != nil {
			return false, nil, err
		}
		h := store.HashBytes(fresh)
		if got, ok := t.Pristine(e.Name); !ok || got != h {
			return false, t.SetPristine(e.Name, h), nil
		}
	}
	return false, nil, nil
}

// PlanEdit devolve a recusa que Prepare aplicaria, sem escrever nada: é o que o
// --dry-run do edit consulta, para a simulação não prometer o que a execução
// real recusa. Só New e Outdated gravam o arquivo; Edited, Current e Custom
// seguem o link, e o editor escreve através dele como o usuário espera.
func (t Templates) PlanEdit(name string) (Entry, error) {
	e, err := t.Lookup(name)
	if err != nil {
		return Entry{}, err
	}
	if e.State == New || e.State == Outdated {
		return e, t.refuseSymlink(e.Name)
	}
	return e, nil
}

// exampleData são os dois conjuntos com que Validate renderiza: com e sem Stack,
// para cobrir os dois ramos de {{if .Stack}}.
var exampleData = []scaffold.Data{
	{ProjectName: "example", Stack: ""},
	{ProjectName: "example", Stack: "example"},
}

// Validate renderiza name (overlay antes do embutido) com os dados de exemplo.
// warnings traz avisos que não são erro (hook com CRLF).
func (t Templates) Validate(name string) (warnings []string, err error) {
	e, err := t.Lookup(name)
	if err != nil {
		return nil, err
	}
	var rendered []byte
	for _, d := range exampleData {
		rendered, err = scaffold.Render(e.Name, t.Dir, d)
		if err != nil {
			return nil, err
		}
	}
	if strings.HasSuffix(e.Name, ".sh.tmpl") && strings.Contains(string(rendered), "\r\n") {
		warnings = append(warnings, "CRLF line endings: the hook will fail in bash")
	}
	return warnings, nil
}
