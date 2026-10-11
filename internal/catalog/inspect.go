package catalog

import (
	"errors"
	"os"
	"path"
	"path/filepath"

	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/scaffold"
	"github.com/TheBud4/ray/internal/store"
)

// Status de um componente na checagem de uma receita.
const (
	ComponentOK      = "ok"
	ComponentMissing = "missing"
	ComponentRefused = "refused"
)

// ComponentReport diz se o `init ai` copiaria um componente da receita.
type ComponentReport struct {
	Name, Dest, Status, Detail string
}

// FileReport diz de qual template vem um arquivo da receita e se ele funciona.
// Problem é "" quando está tudo certo.
type FileReport struct {
	Path, Template string
	State          State
	Problem        string
}

// Report é o resultado de Inspect.
type Report struct {
	Components []ComponentReport
	Files      []FileReport
}

// Problems conta o que faria o `init ai` recusar ou pular algo: componente
// ausente ou recusado, template ausente, arquivo sem template e template que
// não renderiza. Estado de template (edited, outdated…) não é problema.
func (r Report) Problems() int {
	n := 0
	for _, c := range r.Components {
		if c.Status != ComponentOK {
			n++
		}
	}
	for _, f := range r.Files {
		if f.Problem != "" {
			n++
		}
	}
	return n
}

// Inspect confere uma receita sem executar nada: cada componente (existe? a
// origem é aceita?) e cada arquivo que o `init ai` escreveria — os da receita
// mais os de sistema —, com o template de origem e o estado dele.
func (t Templates) Inspect(p *profile.Profile, componentsDir string) (Report, error) {
	var r Report
	for _, c := range p.Components {
		cr := ComponentReport{Name: c.Name, Dest: filepath.Join(c.Dest, c.Name), Status: ComponentOK}
		src := filepath.Join(componentsDir, c.Name)
		if info, err := os.Stat(src); err != nil || !info.IsDir() {
			cr.Status, cr.Detail = ComponentMissing, "not found at "+src
		} else if _, err := store.ResolveSources(componentsDir, []string{c.Name}); err != nil {
			cr.Status, cr.Detail = ComponentRefused, refusalReason(err)
		}
		r.Components = append(r.Components, cr)
	}

	entries, err := t.List()
	if err != nil {
		return Report{}, err
	}
	states := make(map[string]State, len(entries))
	for _, e := range entries {
		states[e.Name] = e.State
	}

	for _, f := range scaffold.DedupFiles(p.Scaffold.Files, scaffold.SystemFiles()) {
		fr := FileReport{Path: f.Path, Template: f.Template}
		if fr.Template == "" {
			name, ok := scaffold.TemplateFor(f.Path)
			if !ok {
				fr.Problem = "no template for path"
				r.Files = append(r.Files, fr)
				continue
			}
			fr.Template = name
		}
		switch {
		case !scaffold.TemplateExists(fr.Template, t.Dir):
			fr.Problem = "missing template"
		default:
			fr.State = states[path.Clean(filepath.ToSlash(fr.Template))]
			for _, d := range exampleData {
				if _, err := scaffold.Render(fr.Template, t.Dir, d); err != nil {
					fr.Problem = "broken: " + err.Error()
					break
				}
			}
		}
		r.Files = append(r.Files, fr)
	}
	return r, nil
}

// refusalReason tira de err o prefixo `refusing to copy component "x":` que o
// store põe, para o motivo não repetir o nome do componente.
func refusalReason(err error) string {
	if inner := errors.Unwrap(err); inner != nil {
		return inner.Error()
	}
	return err.Error()
}
