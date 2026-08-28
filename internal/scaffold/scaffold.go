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
	"slices"
	"strings"
	"text/template"

	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/store"
)

//go:embed templates
var embedded embed.FS

const templatesRoot = "templates"

// handoffPath é o único arquivo de scaffold imune a --force (docs/conventions.md).
const handoffPath = ".claude/handoff.md"

// executableSuffix marca quais arquivos saem 0755 em vez de 0644.
const executableSuffix = ".sh"

// templateFor liga cada path alvo ao seu arquivo .tmpl dentro de templates/.
var templateFor = map[string]string{
	"CLAUDE.md":                      "CLAUDE.md.tmpl",
	"SECURITY.md":                    "SECURITY.md.tmpl",
	"docs/README.md":                 "docs/README.md.tmpl",
	"docs/architecture.md":           "docs/architecture.md.tmpl",
	"docs/conventions.md":            "docs/conventions.md.tmpl",
	".claude/commands/document.md":   "claude/commands/document.md.tmpl",
	".claude/commands/handoff.md":    "claude/commands/handoff.md.tmpl",
	".claude/commands/revisar.md":    "claude/commands/revisar.md.tmpl",
	".claude/commands/destilar.md":   "claude/commands/destilar.md.tmpl",
	".claude/handoff.md":             "claude/handoff.md.tmpl",
	".claude/hooks/session-start.sh": "claude/hooks/session-start.sh.tmpl",
	".claude/hooks/guard-add.sh":     "claude/hooks/guard-add.sh.tmpl",
	".claude/hooks/guard-vocab.sh":   "claude/hooks/guard-vocab.sh.tmpl",
	".claude/hooks/guard-plans.sh":   "claude/hooks/guard-plans.sh.tmpl",
	".claude/hooks/guard-handoff.sh": "claude/hooks/guard-handoff.sh.tmpl",
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
//
// A resolução de template de toda a lista é validada antes de escrever
// qualquer arquivo (RF-04): um path sem template no meio da lista não pode
// deixar os arquivos anteriores já gravados no disco.
func WriteFiles(files []profile.ScaffoldFile, opts Options) (Result, error) {
	tmplNames := make([]string, len(files))
	for i, f := range files {
		tmplName := f.Template
		if tmplName == "" {
			var ok bool
			tmplName, ok = templateFor[f.Path]
			if !ok {
				return Result{}, fmt.Errorf("scaffold: no template for path %q", f.Path)
			}
		}
		// Não basta ter um nome — precisa existir em disco (overlay ou
		// embed), senão um `template:` customizado errado só falhava no
		// loop de escrita abaixo, com os paths anteriores já gravados.
		if _, err := readTemplate(tmplName, opts.TemplatesDir); err != nil {
			return Result{}, err
		}
		tmplNames[i] = tmplName
	}

	var res Result
	for i, f := range files {
		tmplName := tmplNames[i]

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

// readTemplate prefere o overlay e cai para o embed. Um nome ausente dos dois
// nomeia o overlay esperado no erro (RF-05) — sem isso, um template
// customizado nunca criado vazava o erro cru do embed.FS, sem indicar que o
// arquivo precisa existir em templatesDir.
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
	data, err := embedded.ReadFile(templatesRoot + "/" + tmplName)
	if err != nil {
		if templatesDir != "" {
			return nil, fmt.Errorf("template %q not found in %s nor embedded in the binary", tmplName, templatesDir)
		}
		return nil, fmt.Errorf("template %q not found embedded in the binary", tmplName)
	}
	return data, nil
}

const (
	gitignoreMarkerBegin = "# >>> ray"
	gitignoreMarkerEnd   = "# <<< ray"
)

// gitignoreBaseLines é a "regra-mãe" (I1): conteúdo de IA vendorizado é
// commitado (whitelist explícita contra qualquer ignore anterior no arquivo);
// runtime, segredos e material pessoal nunca são (blacklist).
var gitignoreBaseLines = []string{
	"# Conteúdo de IA vendorizado — commitado (não editar as negações abaixo)",
	"!.claude/skills/",
	"!.claude/agents/",
	"!.claude/commands/",
	"!.claude/settings.json",
	"!.mcp.json",
	"!docs/",
	"!.claude/.ray-profile",
	"!**/.ray-origin",
	"!**/LICENSE",
	"",
	"# Runtime, segredos e pessoal — nunca commitados",
	"graphify-out/",
	".claude/.ray-metrics/",
	".claude/.local/",
	".claude/handoff.md",
	".env",
	"*.local",
}

// GitignoreBaseLines devolve uma cópia das linhas do bloco que o ray escreve
// no .gitignore. Existe para o `ray status` poder verificar se o bloco segue
// intacto sem duplicar a lista — duas cópias divergiriam, e a whitelist é o
// que faz o vendoring funcionar.
func GitignoreBaseLines() []string {
	return slices.Clone(gitignoreBaseLines)
}

// GitignoreMarkers devolve os marcadores que delimitam o bloco do ray.
func GitignoreMarkers() (begin, end string) {
	return gitignoreMarkerBegin, gitignoreMarkerEnd
}

// MergeGitignore garante que <target>/.gitignore contenha o bloco do ray
// (whitelist do conteúdo de IA + blacklist de runtime/segredos, mais
// stackLines específicas do perfil, renderizadas com data — ex.
// "/{{.ProjectName}}"), preservando o resto do arquivo. O bloco vive entre
// marcadores idempotentes — rodar de novo não duplica nem perde linhas que o
// usuário tenha fora deles. dryRun imprime o resultado em out em vez de
// gravar.
func MergeGitignore(target string, stackLines []string, data Data, dryRun bool, out io.Writer) error {
	path := filepath.Join(target, ".gitignore")

	existing, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	renderedStack, err := renderGitignoreStackLines(stackLines, data)
	if err != nil {
		return err
	}

	block := buildGitignoreBlock(renderedStack)
	content, changed := mergeMarkedBlock(string(existing), block)

	if dryRun {
		if out != nil {
			_, err := io.WriteString(out, content)
			return err
		}
		return nil
	}
	if !changed {
		return nil
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// renderGitignoreStackLines executa cada linha de stackLines como
// text/template com data (ex. "/{{.ProjectName}}" → "/ray").
func renderGitignoreStackLines(stackLines []string, data Data) ([]string, error) {
	rendered := make([]string, len(stackLines))
	for i, line := range stackLines {
		t, err := template.New("gitignore-stack-line").Parse(line)
		if err != nil {
			return nil, fmt.Errorf("parsing gitignore stack line %q: %w", line, err)
		}
		var buf bytes.Buffer
		if err := t.Execute(&buf, data); err != nil {
			return nil, fmt.Errorf("rendering gitignore stack line %q: %w", line, err)
		}
		rendered[i] = buf.String()
	}
	return rendered, nil
}

// buildGitignoreBlock monta o bloco completo (base + stackLines) entre
// marcadores.
func buildGitignoreBlock(stackLines []string) string {
	lines := append([]string{gitignoreMarkerBegin}, gitignoreBaseLines...)
	if len(stackLines) > 0 {
		lines = append(lines, "")
		lines = append(lines, stackLines...)
	}
	lines = append(lines, gitignoreMarkerEnd)
	return strings.Join(lines, "\n")
}

// mergeMarkedBlock devolve existing com block inserido/substituído entre
// gitignoreMarkerBegin/End, e se o conteúdo mudou. Sem marcador prévio,
// acrescenta ao final; com marcador, substitui só o miolo.
func mergeMarkedBlock(existing, block string) (string, bool) {
	beginIdx := strings.Index(existing, gitignoreMarkerBegin)
	if beginIdx == -1 {
		trimmed := strings.TrimRight(existing, "\n")
		var b strings.Builder
		if trimmed != "" {
			b.WriteString(trimmed)
			b.WriteString("\n\n")
		}
		b.WriteString(block)
		b.WriteString("\n")
		return b.String(), true
	}

	endIdx := strings.Index(existing[beginIdx:], gitignoreMarkerEnd)
	var newContent string
	if endIdx == -1 {
		newContent = existing[:beginIdx] + block + "\n"
	} else {
		endIdx += beginIdx + len(gitignoreMarkerEnd)
		newContent = existing[:beginIdx] + block + existing[endIdx:]
	}
	return newContent, newContent != existing
}

// TemplateAction é o que EnsureTemplates fez com um arquivo do overlay.
type TemplateAction string

const (
	TemplateCreated   TemplateAction = "created"   // não existia
	TemplateRefreshed TemplateAction = "refreshed" // existia intocado e defasou
	TemplateCurrent   TemplateAction = "current"   // existia e já era igual ao embed
	TemplateKept      TemplateAction = "kept"      // editado localmente: preservado
)

// TemplateSync é o resultado por arquivo. O chamador usa Hash para gravar a
// linha-base pristina (só em Created/Refreshed) e Reason para avisar (Kept).
type TemplateSync struct {
	Rel    string
	Action TemplateAction
	Reason string
	Hash   string
}

// EnsureOptions injeta o que EnsureTemplates não tem como saber sozinho.
type EnsureOptions struct {
	// Pristine devolve o hash gravado por último para um template do overlay.
	// Nil ou ok=false cai na degradação graciosa de store.DecideOverwrite.
	Pristine func(rel string) (string, bool)
	// Force sobrescreve mesmo template editado localmente.
	Force bool
}

// EnsureTemplates sincroniza o overlay editável em dir com os templates
// embutidos, e devolve o que fez com cada arquivo.
//
// Antes, ele só criava o que faltava e devolvia sem tocar no resto. Isso fazia
// o overlay **sombrear o embed em silêncio**: gerado por uma versão antiga, ele
// continuava ganhando do binário novo — porque o `render` prefere o overlay —
// mesmo sem ninguém o ter editado. Atualizar o `ray` não atualizava os
// templates, e nada avisava.
//
// A política de "o usuário editou isto?" é store.DecideOverwrite, a mesma do
// `ray update` para conteúdo vendorizado: intocado é reescrito em silêncio,
// editado é preservado com motivo, e --force atropela. Duas perguntas iguais
// não devem ter respostas diferentes.
func EnsureTemplates(dir string, opts EnsureOptions) ([]TemplateSync, error) {
	var synced []TemplateSync
	err := fs.WalkDir(embedded, templatesRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel := strings.TrimPrefix(path, templatesRoot+"/")
		dest := filepath.Join(dir, filepath.FromSlash(rel))

		fresh, err := embedded.ReadFile(path)
		if err != nil {
			return err
		}
		freshHash := store.HashBytes(fresh)

		onDisk, readErr := os.ReadFile(dest)
		exists := readErr == nil
		if readErr != nil && !os.IsNotExist(readErr) {
			return readErr
		}

		// Igual ao embed: nada a fazer, e não é "editado". Sai antes da
		// política para não gerar ruído no caso mais comum de todos.
		onDiskHash := store.HashBytes(onDisk)
		if exists && onDiskHash == freshHash {
			synced = append(synced, TemplateSync{Rel: rel, Action: TemplateCurrent, Hash: freshHash})
			return nil
		}

		var pristineHash string
		var hasPristine bool
		if opts.Pristine != nil {
			pristineHash, hasPristine = opts.Pristine(rel)
		}
		overwrite, reason := store.DecideOverwrite(opts.Force, exists, onDiskHash, freshHash, pristineHash, hasPristine)
		if !overwrite {
			synced = append(synced, TemplateSync{Rel: rel, Action: TemplateKept, Reason: reason})
			return nil
		}

		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(dest, fresh, 0o644); err != nil {
			return err
		}
		action := TemplateCreated
		if exists {
			action = TemplateRefreshed
		}
		synced = append(synced, TemplateSync{Rel: rel, Action: action, Hash: freshHash})
		return nil
	})
	return synced, err
}
