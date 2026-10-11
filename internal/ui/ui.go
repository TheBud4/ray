// Package ui é a camada de apresentação da saída do terminal: o cmd diz o que
// uma linha é (título, seção, item feito, item que falhou, resumo, tabela...) e
// o ui decide como ela aparece, com ou sem cor. Só este pacote conhece escape
// ANSI e o console do Windows; os pacotes de domínio não o importam.
package ui

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// Color é um papel de cor, não um código: o SGR de cada uma está em sgr.
type Color int

// As cores são só as básicas do ANSI, para seguir o tema do terminal.
const (
	Plain Color = iota
	Green
	Yellow
	Red
	Cyan
	Gray
)

// sgr guarda o código SGR de cada cor. Cinza é o 90 (bright black).
var sgr = map[Color]string{
	Green:  "32",
	Yellow: "33",
	Red:    "31",
	Cyan:   "36",
	Gray:   "90",
}

// Kind é o estado de um item de lista; o marcador e a cor saem dele.
type Kind int

const (
	OK      Kind = iota // feito
	Fail                // falhou
	Warn                // atenção
	Added               // criado
	Updated             // atualizado
	Kept                // preservado
)

func (k Kind) marker() string {
	switch k {
	case OK:
		return "✓"
	case Fail:
		return "✗"
	case Warn:
		return "⚠"
	case Added:
		return "+"
	case Updated:
		return "~"
	default:
		return "="
	}
}

func (k Kind) color() Color {
	switch k {
	case OK, Added:
		return Green
	case Fail:
		return Red
	case Warn, Updated:
		return Yellow
	default:
		return Gray
	}
}

// Item é uma linha de lista: um marcador de estado, um nome e um detalhe.
type Item struct {
	Kind         Kind
	Name, Detail string
}

// Count é uma contagem do resumo final; N zero não aparece.
type Count struct {
	N     int
	Label string
	Color Color
}

// Cell é uma célula de tabela.
type Cell struct {
	Text  string
	Color Color
}

// Printer escreve a saída de um comando. Com color falso, é texto simples. Não
// guarda estado entre escritas.
type Printer struct {
	w     io.Writer
	color bool
}

// New devolve um Printer sobre w. Se w já é um Printer, usa o writer de baixo.
func New(w io.Writer, color bool) *Printer {
	if p, ok := w.(*Printer); ok {
		w = p.w
	}
	return &Printer{w: w, color: color}
}

// For devolve w como Printer: o próprio w se já é um, senão um Printer sem cor.
// É assim que as funções run* continuam recebendo um io.Writer comum.
func For(w io.Writer) *Printer {
	if p, ok := w.(*Printer); ok {
		return p
	}
	return &Printer{w: w}
}

// Raw devolve o writer que está sob o Printer (o *os.File original, quando é o
// caso), ou o próprio w se não é um Printer. Todo writer que vai a um processo
// filho passa por aqui: um writer que não é *os.File faz o Go criar um pipe, e
// o filho deixa de ver um terminal.
func Raw(w io.Writer) io.Writer {
	if p, ok := w.(*Printer); ok {
		return p.w
	}
	return w
}

// Write é transparente: escreve os bytes recebidos sem tocar neles. Só os
// métodos semânticos emitem escape.
func (p *Printer) Write(b []byte) (int, error) { return p.w.Write(b) }

// Color diz se este Printer emite cor.
func (p *Printer) Color() bool { return p.color }

// Paint pinta s com c; sem cor, devolve s.
func (p *Printer) Paint(c Color, s string) string {
	if !p.color || c == Plain || s == "" {
		return s
	}
	return "\x1b[" + sgr[c] + "m" + s + "\x1b[0m"
}

// Bold deixa s em negrito; sem cor, devolve s.
func (p *Printer) Bold(s string) string {
	if !p.color || s == "" {
		return s
	}
	return "\x1b[1m" + s + "\x1b[22m"
}

// Line escreve s e uma quebra de linha.
func (p *Printer) Line(s string) { fmt.Fprintln(p.w, s) }

// Blank escreve uma linha em branco.
func (p *Printer) Blank() { fmt.Fprintln(p.w) }

// Title escreve o título de um comando: o nome em negrito e o detalhe apagado.
func (p *Printer) Title(name, detail string) {
	line := p.Bold(name)
	if detail != "" {
		line += " " + p.Paint(Gray, "· "+detail)
	}
	fmt.Fprintln(p.w, line)
}

// Section escreve o nome de uma seção em negrito.
func (p *Printer) Section(name string) { fmt.Fprintln(p.w, p.Bold(name)) }

// Items escreve uma seção de itens. Os nomes são preenchidos até o maior nome
// visível da seção, e o detalhe vem 2 espaços depois. O marcador fica sempre
// na terceira coluna e não entra na conta. A cor é aplicada depois do
// preenchimento.
func (p *Printer) Items(items []Item) {
	width := 0
	for _, it := range items {
		width = max(width, utf8.RuneCountInString(it.Name))
	}
	for _, it := range items {
		marker := p.Paint(it.Kind.color(), it.Kind.marker())
		name := it.Name
		switch it.Kind {
		case Fail:
			name = p.Paint(Red, name)
		case Kept:
			name = p.Paint(Gray, name)
		}
		line := "  " + marker + " " + name
		if it.Detail != "" {
			pad := width - utf8.RuneCountInString(it.Name)
			line += strings.Repeat(" ", pad) + "  " + p.Paint(Gray, it.Detail)
		}
		fmt.Fprintln(p.w, line)
	}
}

// Summary escreve as contagens maiores que zero, na ordem dada, ligadas por
// " · ". Sem nenhuma contagem, não escreve nada.
func (p *Printer) Summary(counts ...Count) {
	var parts []string
	for _, c := range counts {
		if c.N > 0 {
			parts = append(parts, p.Paint(c.Color, fmt.Sprintf("%d %s", c.N, c.Label)))
		}
	}
	if len(parts) > 0 {
		fmt.Fprintln(p.w, strings.Join(parts, p.Paint(Gray, " · ")))
	}
}

// Table escreve um cabeçalho (apagado) e as linhas, com as colunas alinhadas
// pela largura visível e 2 espaços entre elas. A última coluna não é preenchida.
func (p *Printer) Table(header []string, rows [][]Cell) {
	widths := make([]int, len(header))
	for i, h := range header {
		widths[i] = utf8.RuneCountInString(h)
	}
	for _, row := range rows {
		for i, c := range row {
			widths[i] = max(widths[i], utf8.RuneCountInString(c.Text))
		}
	}
	write := func(cells []Cell) {
		var b strings.Builder
		for i, c := range cells {
			b.WriteString(p.Paint(c.Color, c.Text))
			if i < len(cells)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(c.Text)+2))
			}
		}
		// Uma última célula vazia (a dica de uma linha sem dica) não deixa
		// espaço sobrando no fim da linha.
		fmt.Fprintln(p.w, strings.TrimRight(b.String(), " "))
	}
	head := make([]Cell, len(header))
	for i, h := range header {
		head[i] = Cell{Text: h, Color: Gray}
	}
	write(head)
	for _, row := range rows {
		write(row)
	}
}

// DiffLine escreve uma linha de diff unificado colorida pelo prefixo.
func (p *Printer) DiffLine(line string) {
	c := Plain
	switch {
	case strings.HasPrefix(line, "+++ "), strings.HasPrefix(line, "--- "):
		c = Gray
	case strings.HasPrefix(line, "@@"):
		c = Cyan
	case strings.HasPrefix(line, "+"):
		c = Green
	case strings.HasPrefix(line, "-"):
		c = Red
	}
	fmt.Fprintln(p.w, p.Paint(c, line))
}

// Error escreve uma mensagem de erro: só a primeira linha leva o marcador e o
// vermelho; as demais saem como estão. A quebra de linha final é aparada.
func (p *Printer) Error(msg string) {
	msg = strings.TrimRight(msg, "\n")
	first, rest, more := strings.Cut(msg, "\n")
	fmt.Fprintln(p.w, p.Paint(Red, "✗ "+first))
	if more {
		fmt.Fprintln(p.w, rest)
	}
}

// Result escreve o resultado de uma ação numa linha, sem recuo:
// "✓ nome  detalhe". É a linha com que um comando de uma ação só termina.
func (p *Printer) Result(kind Kind, name, detail string) {
	if kind == Fail {
		name = p.Paint(Red, name)
	}
	line := p.Paint(kind.color(), kind.marker()) + " " + name
	if detail != "" {
		line += "  " + p.Paint(Gray, detail)
	}
	fmt.Fprintln(p.w, line)
}

// Warning escreve um aviso: "⚠ manchete" e, se houver, a dica numa linha
// recuada e apagada embaixo.
func (p *Printer) Warning(headline, hint string) {
	fmt.Fprintln(p.w, p.Paint(Yellow, "⚠")+" "+headline)
	if hint != "" {
		fmt.Fprintln(p.w, "    "+p.Paint(Gray, hint))
	}
}

// Verdict escreve a linha final de um diagnóstico: o marcador e o texto na cor
// do estado, e o detalhe, se houver, depois de " — ".
func (p *Printer) Verdict(kind Kind, text, detail string) {
	line := p.Paint(kind.color(), kind.marker()+" "+text)
	if detail != "" {
		line += " " + p.Paint(Gray, "— "+detail)
	}
	fmt.Fprintln(p.w, line)
}

// Steps escreve a lista de próximos passos: o comando em ciano e a descrição,
// com os comandos preenchidos até o maior da lista mais 2 espaços.
func (p *Printer) Steps(steps [][2]string) {
	width := 0
	for _, s := range steps {
		width = max(width, utf8.RuneCountInString(s[0]))
	}
	for _, s := range steps {
		line := "  " + p.Paint(Cyan, s[0])
		if s[1] != "" {
			line += strings.Repeat(" ", width-utf8.RuneCountInString(s[0])) + "  " + s[1]
		}
		fmt.Fprintln(p.w, line)
	}
}

// ShortenHome troca o prefixo da pasta home por "~" em s, para exibir um
// caminho. Só vale onde o prefixo termina num separador de caminho ou no fim do
// texto; no mais, s volta como está. Só na exibição: o domínio guarda o caminho
// inteiro.
func ShortenHome(s, home string) string {
	if home == "" {
		return s
	}
	var b strings.Builder
	for {
		i := strings.Index(s, home)
		if i < 0 {
			break
		}
		end := i + len(home)
		if end < len(s) && s[end] != '/' && s[end] != '\\' {
			b.WriteString(s[:end])
			s = s[end:]
			continue
		}
		b.WriteString(s[:i])
		b.WriteString("~")
		s = s[end:]
	}
	b.WriteString(s)
	return b.String()
}
