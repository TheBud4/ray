// Package status implementa `ray status` (I8): diagnostica o ambiente de IA
// vendorizado num projeto. Só lê — nunca grava, e nunca vai à rede.
package status

import (
	"os"
	"path/filepath"

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
	return run(check, preflight.RunnerLooker{Runner: runner.ExecRunner{}}, opts, home)
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

	name, forks, forkProblems, err := checkForks(check, target, home)
	if err != nil {
		return Report{}, err
	}
	rep.Profile = name
	rep.Forks = forks
	rep.Problems = append(rep.Problems, forkProblems...)

	rep.Git, rep.DirtyN, rep.AddPaths = checkGit(check, target)

	gitignoreProblems, err := checkGitignore(target)
	if err != nil {
		return Report{}, err
	}
	rep.Problems = append(rep.Problems, gitignoreProblems...)

	n, mcpProblems, err := checkMCP(l, target)
	if err != nil {
		return Report{}, err
	}
	rep.Inventory.MCPServers = n
	rep.Problems = append(rep.Problems, mcpProblems...)

	return rep, nil
}

// countInventory conta as entradas de topo de cada diretório de conteúdo.
// Diretório ausente conta zero — o ambiente pode legitimamente não ter agents.
func countInventory(target string) (Inventory, error) {
	var inv Inventory
	for _, pair := range []struct {
		dir string
		n   *int
	}{
		{"skills", &inv.Skills},
		{"agents", &inv.Agents},
		{"commands", &inv.Commands},
	} {
		entries, err := os.ReadDir(filepath.Join(claudeDir(target), pair.dir))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return Inventory{}, err
		}
		*pair.n = len(entries)
	}
	return inv, nil
}
