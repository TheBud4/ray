package status

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/scaffold"
)

// gitScopeDenylist são os caminhos da whitelist que o status manda commitar
// mas não vigia. docs/ é do usuário: flagrar edição dele como drift do
// ambiente seria falso positivo. CLAUDE.md nem está na whitelist.
var gitScopeDenylist = map[string]bool{"docs": true}

// gitScope são os caminhos do ambiente que o status observa, derivados da
// mesma whitelist do .gitignore que define o que é ambiente vendorizado.
//
// Derivado, e não fixo, porque a lista fixa dessincronizava em silêncio: a
// whitelist ganha uma entrada e o status simplesmente para de vigiar parte do
// ambiente que ele mesmo manda commitar, sem nada falhar. O que sai fica no
// denylist acima, onde a exclusão é visível.
func gitScope() []string {
	var out []string
	for _, p := range whitelistTopLevel() {
		if gitScopeDenylist[p] {
			continue
		}
		out = append(out, p)
	}
	return out
}

// errNotARepo marca exit ≠ 0 do git. Não vaza para o chamador de Run — vira
// GitUnavailable.
var errNotARepo = errors.New("git reported a non-zero exit")

// checkGit resolve o estado do ambiente na árvore com duas consultas.
//
// A primeira (ls-files) distingue "nunca versionei" de "versionei e depois
// divergiu" — distinção que importa porque logo depois do `ray init ai` tudo
// está untracked, e isso é o estado normal, não um problema. Adivinhar isso
// pelos códigos do porcelain seria frágil; o ls-files responde direto.
//
// Qualquer falha (não é repo, git ausente) devolve GitUnavailable sem erro: a
// seção some e o resto do diagnóstico continua.
func checkGit(check runner.Runner, target string) (GitState, int, []string) {
	if check == nil {
		return GitUnavailable, 0, nil
	}

	tracked, err := gitOut(check, target, append([]string{"ls-files", "--"}, gitScope()...))
	if err != nil {
		return GitUnavailable, 0, nil
	}
	if strings.TrimSpace(tracked) == "" {
		return GitNeverTracked, 0, addPaths(target)
	}

	porcelain, err := gitOut(check, target, append([]string{"status", "--porcelain", "--"}, gitScope()...))
	if err != nil {
		return GitUnavailable, 0, nil
	}
	if n := countLines(porcelain); n > 0 {
		return GitDirty, n, nil
	}
	return GitClean, 0, nil
}

func gitOut(check runner.Runner, dir string, args []string) (string, error) {
	res, err := check.Run(context.Background(), runner.Command{Name: "git", Args: args, Dir: dir})
	if err != nil {
		return "", err
	}
	if res.ExitCode != 0 {
		return "", errNotARepo
	}
	return res.Stdout, nil
}

func countLines(s string) int {
	n := 0
	for _, l := range strings.Split(s, "\n") {
		if strings.TrimSpace(l) != "" {
			n++
		}
	}
	return n
}

// addPaths são os caminhos de topo do ambiente que existem em disco, para o
// `git add` da nota. Deliberadamente por nome, nunca `-A` nem `.`: o
// guard-add.sh que o próprio ray instala avisa contra add cego.
func addPaths(target string) []string {
	out := environmentTopLevel(target)
	sort.Strings(out)
	return out
}

// environmentTopLevel devolve os caminhos de topo do ambiente presentes em
// disco. É a whitelist inteira, sem o denylist do gitScope: o `git add` da
// nota tem que oferecer docs/ também, que é conteúdo vendorizado a commitar
// ainda que o status não o vigie depois.
func environmentTopLevel(target string) []string {
	var out []string
	for _, p := range whitelistTopLevel() {
		if _, err := os.Stat(filepath.Join(target, p)); err != nil {
			continue
		}
		out = append(out, p)
	}
	return out
}

// whitelistTopLevel devolve os caminhos de topo das negações do bloco do
// .gitignore — a whitelist que define o que é ambiente vendorizado, e a única
// fonte de verdade sobre isso. Padrões com glob (`**/…`) ficam de fora: não
// são caminho de topo, e não servem nem como pathspec de git nem para
// `git add`.
func whitelistTopLevel() []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range scaffold.GitignoreBaseLines() {
		if !strings.HasPrefix(l, "!") || strings.Contains(l, "*") {
			continue
		}
		p := strings.TrimSuffix(strings.TrimPrefix(l, "!"), "/")
		if i := strings.IndexByte(p, '/'); i > 0 {
			p = p[:i]
		}
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}
