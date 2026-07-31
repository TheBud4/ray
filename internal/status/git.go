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

// gitScope são os caminhos do ambiente que o status observa. docs/ e CLAUDE.md
// ficam de fora de propósito: são do usuário, e flagrar edição deles como
// drift do ambiente seria falso positivo.
var gitScope = []string{".claude", ".mcp.json"}

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

	tracked, err := gitOut(check, target, append([]string{"ls-files", "--"}, gitScope...))
	if err != nil {
		return GitUnavailable, 0, nil
	}
	if strings.TrimSpace(tracked) == "" {
		return GitNeverTracked, 0, addPaths(target)
	}

	porcelain, err := gitOut(check, target, append([]string{"status", "--porcelain", "--"}, gitScope...))
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
	seen := map[string]bool{}
	for _, p := range environmentTopLevel(target) {
		seen[p] = true
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// environmentTopLevel devolve os caminhos de topo do ambiente presentes em
// disco, derivados das negações do bloco do .gitignore — a mesma whitelist que
// define o que é ambiente vendorizado. Padrões com glob (`**/…`) ficam de
// fora: não são caminho de topo e não servem para `git add`.
func environmentTopLevel(target string) []string {
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
		if _, err := os.Stat(filepath.Join(target, p)); err != nil {
			continue
		}
		seen[p] = true
		out = append(out, p)
	}
	return out
}
