// Package scaffold escreve a árvore de orientação (Agent Development Kit) na
// pasta-alvo: CLAUDE.md, SECURITY.md, docs/, .claude/rules, hooks de sistema.
// Só escreve arquivos — não chama processos.
package scaffold

import (
	"bytes"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/TheBud4/ray/internal/profile"
)

//go:embed templates
var embedded embed.FS

const templatesRoot = "templates"

// handoffPath é o único arquivo de scaffold imune a --force (§2 do build guide).
const handoffPath = ".claude/handoff.md"

// executableSuffix marca quais arquivos saem 0755 em vez de 0644.
const executableSuffix = ".sh"

// templateFor liga cada path alvo ao seu arquivo .tmpl dentro de templates/.
var templateFor = map[string]string{
	"CLAUDE.md":                        "CLAUDE.md.tmpl",
	"SECURITY.md":                      "SECURITY.md.tmpl",
	"docs/README.md":                   "docs/README.md.tmpl",
	"docs/architecture.md":             "docs/architecture.md.tmpl",
	"docs/conventions.md":              "docs/conventions.md.tmpl",
	"docs/context.md":                  "docs/context.md.tmpl",
	".claude/rules/security.md":        "claude/rules/security.md.tmpl",
	".claude/rules/workflow.md":        "claude/rules/workflow.md.tmpl",
	".claude/rules/handoff.md":         "claude/rules/handoff.md.tmpl",
	".claude/rules/knowledge-vault.md": "claude/rules/knowledge-vault.md.tmpl",
	".claude/rules/documentation.md":   "claude/rules/documentation.md.tmpl",
	".claude/commands/document.md":     "claude/commands/document.md.tmpl",
	".claude/handoff.md":               "claude/handoff.md.tmpl",
	".claude/hooks/session-start.sh":   "claude/hooks/session-start.sh.tmpl",
	".claude/rules/learn.md":           "claude/rules/learn.md.tmpl",
	".claude/hooks/guard-code.sh":      "claude/hooks/guard-code.sh.tmpl",
}

// Data são os placeholders disponíveis em todo template.
type Data struct {
	ProjectName string
	Stack       string
}

// Options controla como WriteFiles escreve em Target.
type Options struct {
	Target       string
	Data         Data
	Force        bool
	DryRun       bool
	Out          io.Writer
	TemplatesDir string // overlay editável (~/.ray/templates); "" = só embed
}

// Result acumula o que WriteFiles fez.
type Result struct {
	Created []string
	Skipped []string
}

// WriteFiles resolve cada f.Template (ou templateFor[f.Path]), renderiza com
// opts.Data e grava em opts.Target/f.Path. Não sobrescreve arquivos existentes
// a menos que opts.Force — exceto .claude/handoff.md, que Force nunca toca.
func WriteFiles(files []profile.ScaffoldFile, opts Options) (Result, error) {
	var res Result
	for _, f := range files {
		tmplName := f.Template
		if tmplName == "" {
			var ok bool
			tmplName, ok = templateFor[f.Path]
			if !ok {
				return Result{}, fmt.Errorf("scaffold: no template for path %q", f.Path)
			}
		}

		targetPath := filepath.Join(opts.Target, f.Path)
		exists := false
		if _, err := os.Stat(targetPath); err == nil {
			exists = true
		} else if !os.IsNotExist(err) {
			return Result{}, err
		}

		force := opts.Force && f.Path != handoffPath
		if exists && !force {
			res.Skipped = append(res.Skipped, f.Path)
			continue
		}

		data, err := render(tmplName, opts.TemplatesDir, opts.Data)
		if err != nil {
			return Result{}, err
		}

		if opts.DryRun {
			if opts.Out != nil {
				fmt.Fprintf(opts.Out, "+ %s\n", f.Path)
			}
			res.Created = append(res.Created, f.Path)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
			return Result{}, err
		}
		mode := os.FileMode(0o644)
		if strings.HasSuffix(f.Path, executableSuffix) {
			mode = 0o755
		}
		if err := os.WriteFile(targetPath, data, mode); err != nil {
			return Result{}, err
		}
		res.Created = append(res.Created, f.Path)
	}
	return res, nil
}

// render resolve tmplName preferindo o overlay (templatesDir/tmplName) e
// caindo para o embed; então executa como text/template com data.
func render(tmplName, templatesDir string, data Data) ([]byte, error) {
	raw, err := readTemplate(tmplName, templatesDir)
	if err != nil {
		return nil, err
	}
	t, err := template.New(tmplName).Parse(string(raw))
	if err != nil {
		return nil, fmt.Errorf("parsing template %s: %w", tmplName, err)
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("rendering template %s: %w", tmplName, err)
	}
	return buf.Bytes(), nil
}

func readTemplate(tmplName, templatesDir string) ([]byte, error) {
	if templatesDir != "" {
		overlay := filepath.Join(templatesDir, tmplName)
		data, err := os.ReadFile(overlay)
		if err == nil {
			return data, nil
		}
		if !os.IsNotExist(err) {
			return nil, err
		}
	}
	return embedded.ReadFile(templatesRoot + "/" + tmplName)
}

// EnsureTemplates copia os templates embutidos para dir como overlay editável,
// nunca sobrescrevendo o que já existir.
func EnsureTemplates(dir string) error {
	return fs.WalkDir(embedded, templatesRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(path, templatesRoot+"/")
		dest := filepath.Join(dir, filepath.FromSlash(rel))
		if _, err := os.Stat(dest); err == nil {
			return nil // já existe: respeita
		}
		data, err := embedded.ReadFile(path)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0o644)
	})
}
