// Package status implementa `ray status` (I8): diagnostica o ambiente de IA
// vendorizado num projeto. Só lê — nunca grava, nunca vai à rede e não executa
// binário de terceiro: a presença dos comandos do .mcp.json vem de
// preflight.PathLooker, um lookup no PATH, e não de rodar `--version` em cada
// um deles. As consultas ao git, que passam pelo runner, são a única execução,
// e são de leitura.
package status

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/TheBud4/ray/internal/mcp"
	"github.com/TheBud4/ray/internal/preflight"
	"github.com/TheBud4/ray/internal/runner"
)

// Options são os parâmetros de Run.
type Options struct {
	Target string
}

// Home reúne os caminhos de ~/.ray que Run precisa, resolvidos pelo chamador
// (via internal/raypaths) para manter este pacote livre de os.Getenv.
type Home struct {
	ProfilesDir string
	StoreDir    string
}

// GitState é o estado do ambiente vendorizado na árvore git.
type GitState int

const (
	// GitUnavailable: não é repositório git, ou o binário git não existe.
	GitUnavailable GitState = iota
	// GitNeverTracked: nenhum arquivo do ambiente está versionado. É o
	// estado normal logo depois do `ray init ai`, e por isso vira nota, não
	// problema.
	GitNeverTracked
	// GitDirty: o ambiente está versionado e divergiu.
	GitDirty
	// GitClean: versionado e sem mudança pendente.
	GitClean
)

// ForkState diz o que o `ray update` faria com um componente vendorizado.
type ForkState int

const (
	// ForkPristine: disco == linha-base; o update atualiza.
	ForkPristine ForkState = iota
	// ForkEdited: disco != linha-base; o update preserva.
	ForkEdited
	// ForkUnknown: não há linha-base para comparar, e sem ela nada pode ser
	// afirmado offline — store.DecideOverwrite precisaria do hash upstream.
	ForkUnknown
)

// ComponentState é o veredito de fork de um componente.
type ComponentState struct {
	Coord string
	State ForkState
}

// Inventory são os fatos: o que existe no ambiente.
type Inventory struct {
	Skills     int
	Agents     int
	Commands   int
	MCPServers int
}

// Report é o resultado de Run — dado puro. Quem decide como pintar é o
// internal/cmd.
type Report struct {
	Profile   string
	Inventory Inventory
	Git       GitState
	DirtyN    int      // arquivos divergentes, quando Git == GitDirty
	AddPaths  []string // caminhos do `git add`, quando Git == GitNeverTracked
	Forks     []ComponentState
	Notes     []string
	Problems  []string
}

// claudeDir é a raiz do ambiente vendorizado dentro do projeto.
func claudeDir(target string) string { return filepath.Join(target, ".claude") }

// Run diagnostica o ambiente em opts.Target. check executa as consultas ao
// git e pode ser nil quando não há git a consultar.
//
// Devolve erro só em falha real de leitura. Problema detectado no ambiente é
// conteúdo do Report, não erro: um comando de diagnóstico que sai ≠ 0 por ter
// achado o que foi procurar é inútil em qualquer script que o encadeie.
func Run(check runner.Runner, opts Options, home Home) (Report, error) {
	return run(check, preflight.PathLooker{}, opts, home)
}

// run é a forma injetável de Run: l resolve presença no PATH. Existe separada
// porque um teste que consulta o PATH real da máquina não é determinístico.
func run(check runner.Runner, l preflight.Looker, opts Options, home Home) (Report, error) {
	var rep Report

	target, err := filepath.Abs(opts.Target)
	if err != nil {
		return Report{}, err
	}

	// Sem .claude/ não há ambiente: uma frase e para. Rodar as quatro
	// checagens aqui só produziria quatro respostas vazias.
	if _, err := os.Stat(claudeDir(target)); err != nil {
		if !os.IsNotExist(err) {
			return Report{}, err
		}
		rep.Problems = append(rep.Problems, "no ray environment here (no .claude/)")
		return rep, nil
	}

	inv, err := countInventory(target)
	if err != nil {
		return Report{}, err
	}
	rep.Inventory = inv

	// Aqui o .mcp.json ilegível é erro, e não silêncio: o `status` existe para
	// diagnosticar, e engolir o arquivo quebrado seria o comando dizendo "tudo
	// em ordem" sobre justamente o que está errado.
	rep.Inventory.MCPServers, err = countMCPServers(target)
	if err != nil {
		return Report{}, err
	}

	name, forks, forkProblems, err := checkForks(target, home)
	if err != nil {
		return Report{}, err
	}
	rep.Profile = name
	rep.Forks = forks
	rep.Problems = append(rep.Problems, forkProblems...)

	rep.Git, rep.DirtyN, rep.AddPaths = checkGit(check, target, home)

	gitignoreProblems, err := checkGitignore(target)
	if err != nil {
		return Report{}, err
	}
	rep.Problems = append(rep.Problems, gitignoreProblems...)

	mcpProblems, err := checkMCP(l, target)
	if err != nil {
		return Report{}, err
	}
	rep.Problems = append(rep.Problems, mcpProblems...)

	return rep, nil
}

// countInventory conta o conteúdo de cada diretório do ambiente.
//
// Conta arquivo, não entrada de topo: uma skill é um SKILL.md, um agente e um
// comando são um .md. Contar `len(os.ReadDir)` fazia um README solto valer uma
// skill e um grupo de três comandos com namespace valer um comando.
//
// Não conta os servidores MCP: isso é countMCPServers, separado porque os dois
// chamadores discordam sobre o que fazer quando o .mcp.json não parseia.
func countInventory(target string) (Inventory, error) {
	var inv Inventory

	for _, pair := range []struct {
		dir   string
		n     *int
		match func(name string) bool
	}{
		{"skills", &inv.Skills, func(name string) bool { return name == "SKILL.md" }},
		{"agents", &inv.Agents, isMarkdown},
		{"commands", &inv.Commands, isMarkdown},
	} {
		n, err := countFiles(filepath.Join(claudeDir(target), pair.dir), pair.match)
		if err != nil {
			return Inventory{}, err
		}
		*pair.n = n
	}
	return inv, nil
}

// countMCPServers conta os servidores declarados no .mcp.json. É o inventário,
// não o diagnóstico: quantos são se responde lendo o arquivo, e é por isso que
// não mora no checkMCP, que precisa do Looker para dizer se o comando de cada
// um está no PATH. Enquanto a contagem morava lá, a tela de `ray` sem
// subcomando — que não paga MCP — imprimia um inventário sem servidores e
// discordava do `ray status` sobre o mesmo projeto.
func countMCPServers(target string) (int, error) {
	servers, err := mcp.ReadServers(target)
	if err != nil {
		return 0, err
	}
	return len(servers), nil
}

func isMarkdown(name string) bool { return strings.EqualFold(filepath.Ext(name), ".md") }

// countFiles conta os arquivos sob root cujo nome casa com match, descendo em
// subdiretórios: skill vendorizada mora em skills/<nome>/SKILL.md, e comando
// com namespace mora em commands/<grupo>/<nome>.md. Diretório ausente conta
// zero — o ambiente pode legitimamente não ter agents.
func countFiles(root string, match func(name string) bool) (int, error) {
	n := 0
	err := filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() && match(d.Name()) {
			n++
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return 0, err
	}
	return n, nil
}
