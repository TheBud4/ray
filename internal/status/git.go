package status

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/TheBud4/ray/internal/profile"
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
func checkGit(check runner.Runner, target string, home Home) (GitState, int, []string) {
	if check == nil {
		return GitUnavailable, 0, nil
	}

	tracked, err := gitOut(check, target, append([]string{"ls-files", "--"}, gitScope()...))
	if err != nil {
		return GitUnavailable, 0, nil
	}
	if strings.TrimSpace(tracked) == "" {
		// A receita só é lida aqui: fora do estado never-tracked não há
		// `git add` a montar, e o status não deve pagar por ela à toa.
		return GitNeverTracked, 0, addPaths(target, scaffoldTopLevel(home, target))
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
//
// A whitelist do .gitignore sozinha não basta, e a razão é estrutural: ela só
// lista o que precisa de **negação**, e o que não é ignorado por ninguém nunca
// aparece lá. Faltavam o `.gitignore` — cujas negações são o que faz o
// vendorizado ser commitado por quem clona, então fora do add o ambiente não
// viaja — e os arquivos que a receita manda escrever na raiz (CLAUDE.md,
// SECURITY.md). O resultado era o `ray status` mandando commitar menos que o
// `ray new` para o mesmo projeto.
func addPaths(target string, scaffoldPaths []string) []string {
	seen := map[string]bool{}
	for _, p := range environmentTopLevel(target) {
		seen[p] = true
	}
	// O .gitignore não vem de lista nenhuma: o ray sempre o escreve, com ou
	// sem receita legível.
	for _, p := range append([]string{".gitignore"}, scaffoldPaths...) {
		if _, err := os.Stat(filepath.Join(target, p)); err != nil {
			continue
		}
		seen[p] = true
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// scaffoldTopLevel são os caminhos de topo que a receita manda escrever.
//
// Carrega a receita por conta própria em vez de reaproveitar a do checkForks
// porque as duas querem coisas opostas do mesmo erro: lá, receita ilegível é
// problema a reportar; aqui é silêncio. O `git add` da nota não pode encolher
// porque o .ray-profile sumiu — o `.gitignore` entra de qualquer jeito.
func scaffoldTopLevel(home Home, target string) []string {
	prof, err := profile.LoadForTarget(home.ProfilesDir, target, "")
	if err != nil {
		return nil
	}
	var out []string
	for _, f := range prof.Scaffold.Files {
		if filepath.IsAbs(f.Path) {
			continue
		}
		p := filepath.ToSlash(filepath.Clean(f.Path))
		if p == "." || p == ".." || strings.HasPrefix(p, "../") {
			continue
		}
		if i := strings.IndexByte(p, '/'); i > 0 {
			p = p[:i]
		}
		out = append(out, p)
	}
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
