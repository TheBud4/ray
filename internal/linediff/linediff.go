// Package linediff produz um diff unificado entre dois textos. É uma função
// pura, sem disco nem processo: o `diff` do sistema não existe no Windows e o
// runner é a única fronteira para processo externo.
package linediff

import (
	"errors"
	"fmt"
	"strings"
)

// maxCells limita n·m do LCS: uma tabela de 5.000×5.000 passaria de 200 MB.
const maxCells = 4_000_000

// context é o número de linhas iguais mostradas em volta de cada mudança.
const context = 3

// noEOL marca, internamente, a última linha de um texto sem "\n" final: assim
// "x" e "x\n" são linhas diferentes, como num diff de verdade.
const noEOL = "\x00noeol"

// ErrTooLarge é devolvido quando o produto das linhas excede o limite.
var ErrTooLarge = errors.New("files are too large to diff")

type op struct {
	kind byte // ' ', '-' ou '+'
	text string
}

// Unified devolve o diff unificado de a para b, ou "" se são iguais. O fim de
// linha "\r" é conteúdo, não separador.
func Unified(a, b, labelA, labelB string) (string, error) {
	if a == b {
		return "", nil
	}
	la, lb := split(a), split(b)

	pre := 0
	for pre < len(la) && pre < len(lb) && la[pre] == lb[pre] {
		pre++
	}
	suf := 0
	for suf < len(la)-pre && suf < len(lb)-pre && la[len(la)-1-suf] == lb[len(lb)-1-suf] {
		suf++
	}
	midA, midB := la[pre:len(la)-suf], lb[pre:len(lb)-suf]
	if len(midA)*len(midB) > maxCells {
		return "", ErrTooLarge
	}

	var ops []op
	for _, l := range la[:pre] {
		ops = append(ops, op{' ', l})
	}
	ops = append(ops, lcs(midA, midB)...)
	for _, l := range la[len(la)-suf:] {
		ops = append(ops, op{' ', l})
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "--- %s\n+++ %s\n", labelA, labelB)
	for _, h := range hunks(ops) {
		writeHunk(&sb, ops, h)
	}
	return sb.String(), nil
}

// split quebra em linhas; a última sem "\n" final leva a marca noEOL.
func split(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	if lines[len(lines)-1] == "" {
		return lines[:len(lines)-1]
	}
	lines[len(lines)-1] += noEOL
	return lines
}

// lcs devolve o script de edição de a para b pela subsequência comum mais longa.
func lcs(a, b []string) []op {
	n, m := len(a), len(b)
	dp := make([]int32, (n+1)*(m+1))
	at := func(i, j int) *int32 { return &dp[i*(m+1)+j] }
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				*at(i, j) = *at(i+1, j+1) + 1
			} else {
				*at(i, j) = max(*at(i+1, j), *at(i, j+1))
			}
		}
	}
	var ops []op
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, op{' ', a[i]})
			i++
			j++
		case *at(i+1, j) >= *at(i, j+1):
			ops = append(ops, op{'-', a[i]})
			i++
		default:
			ops = append(ops, op{'+', b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, op{'-', a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, op{'+', b[j]})
	}
	return ops
}

type hunk struct{ from, to int } // [from, to) em ops

// hunks agrupa as mudanças com `context` linhas de cada lado, juntando as que
// se tocam.
func hunks(ops []op) []hunk {
	var out []hunk
	for i := 0; i < len(ops); i++ {
		if ops[i].kind == ' ' {
			continue
		}
		from := max(i-context, 0)
		j := i
		for j < len(ops) {
			if ops[j].kind != ' ' {
				j++
				continue
			}
			k := j
			for k < len(ops) && ops[k].kind == ' ' {
				k++
			}
			if k < len(ops) && k-j <= 2*context {
				j = k
				continue
			}
			break
		}
		to := min(j+context, len(ops))
		out = append(out, hunk{from, to})
		i = to - 1
	}
	return out
}

func writeHunk(sb *strings.Builder, ops []op, h hunk) {
	aStart, bStart := 1, 1
	for _, o := range ops[:h.from] {
		if o.kind != '+' {
			aStart++
		}
		if o.kind != '-' {
			bStart++
		}
	}
	aLen, bLen := 0, 0
	for _, o := range ops[h.from:h.to] {
		if o.kind != '+' {
			aLen++
		}
		if o.kind != '-' {
			bLen++
		}
	}
	if aLen == 0 {
		aStart--
	}
	if bLen == 0 {
		bStart--
	}
	fmt.Fprintf(sb, "@@ -%s +%s @@\n", rng(aStart, aLen), rng(bStart, bLen))
	for _, o := range ops[h.from:h.to] {
		text, eol := strings.CutSuffix(o.text, noEOL)
		fmt.Fprintf(sb, "%c%s\n", o.kind, text)
		if eol {
			sb.WriteString("\\ No newline at end of file\n")
		}
	}
}

func rng(start, n int) string {
	if n == 1 {
		return fmt.Sprint(start)
	}
	return fmt.Sprintf("%d,%d", start, n)
}
