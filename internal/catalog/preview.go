package catalog

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/TheBud4/ray/internal/scaffold"
)

// PreviewCeiling é o teto, em bytes, da saída de um Preview: um rascunho com
// {{template}} recursivo não pode esgotar a memória de quem o renderiza.
const PreviewCeiling = 4 << 20

// RenderError é uma falha de parse ou de execução do rascunho. Line e Col são 0
// quando o erro não aponta uma posição.
type RenderError struct {
	Line, Col int
	Message   string
}

// PreviewResult é o que um rascunho renderiza: Output, ou Err quando não renderiza.
type PreviewResult struct {
	Output string
	Err    *RenderError // nil quando renderiza
}

// errPos casa o trecho "template: <nome>:<linha>[:<col>]:" do text/template.
var errPos = regexp.MustCompile(`template: [^:]+:(\d+)(?::(\d+))?:`)

// Preview renderiza o rascunho content como o template name, sem gravar nada;
// stack escolhe o conjunto de dados de exemplo (vazio ou preenchido). Uma falha
// do rascunho volta em PreviewResult.Err; só um nome fora da lista é error.
func (t Templates) Preview(name string, content []byte, stack bool) (PreviewResult, error) {
	e, err := t.Lookup(name)
	if err != nil {
		return PreviewResult{}, err
	}
	data := exampleData[0]
	if stack {
		data = exampleData[1]
	}
	out, err := scaffold.RenderString(e.Name, content, data, PreviewCeiling)
	if err != nil {
		return PreviewResult{Err: renderError(e.Name, err)}, nil
	}
	return PreviewResult{Output: string(out)}, nil
}

// renderError traduz o erro do text/template: tira o prefixo de contexto do
// ray e extrai a posição.
func renderError(name string, err error) *RenderError {
	msg := err.Error()
	for _, p := range []string{"parsing template " + name + ": ", "rendering template " + name + ": "} {
		msg = strings.TrimPrefix(msg, p)
	}
	re := &RenderError{Message: msg}
	if errors.Is(err, scaffold.ErrOutputTooLarge) {
		re.Message = fmt.Sprintf("rendered output is larger than the %d MiB ceiling", PreviewCeiling>>20)
		return re
	}
	if m := errPos.FindStringSubmatch(msg); m != nil {
		re.Line, _ = strconv.Atoi(m[1])
		re.Col, _ = strconv.Atoi(m[2]) // vazio → 0
	}
	return re
}

// TemplateFile é um template como o editor o lê. Content vem com CRLF
// normalizado para LF; EOL guarda o que o arquivo tinha, para a gravação
// restaurá-lo.
type TemplateFile struct {
	Name     string
	State    State
	Content  string
	EOL      string // "lf" ou "crlf"
	UTF8     bool
	Hash     string // sha256 hex de Read(name): a base que a gravação confere
	ReadOnly bool   // IsHook(Name)
}

// IsHook diz se name é um hook (.sh.tmpl, sem diferenciar maiúsculas: no
// macOS e no Windows outra caixa é o mesmo arquivo): o editor só o mostra,
// porque CRLF quebra o bash.
func IsHook(name string) bool {
	return strings.HasSuffix(strings.ToLower(name), ".sh.tmpl")
}

// Info lê name pelo mesmo caminho que o próximo `init ai` usaria.
func (t Templates) Info(name string) (TemplateFile, error) {
	e, err := t.Lookup(name)
	if err != nil {
		return TemplateFile{}, err
	}
	raw, err := t.Read(e.Name)
	if err != nil {
		return TemplateFile{}, err
	}
	sum := sha256.Sum256(raw)
	eol := "lf"
	if bytes.Contains(raw, []byte("\r\n")) {
		eol = "crlf"
	}
	return TemplateFile{
		Name:     e.Name,
		State:    e.State,
		Content:  strings.ReplaceAll(string(raw), "\r\n", "\n"),
		EOL:      eol,
		UTF8:     utf8.Valid(raw),
		Hash:     hex.EncodeToString(sum[:]),
		ReadOnly: IsHook(e.Name),
	}, nil
}
