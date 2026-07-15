// Package installer traduz uma receita validada (*profile.Profile) num Plan:
// dados puros (comandos, globais, servers) que uma fase posterior executa.
// Não toca rede, não executa processos, não sabe de CLI.
package installer

import (
	"fmt"

	"github.com/TheBud4/ray/internal/mcp"
	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/runner"
)

// Plan é o resultado determinístico de resolver uma receita.
// Commands: por-projeto, sempre rodam. Globals: install-once, rastreados por
// Key numa fase posterior. Servers: entradas de .mcp.json.
type Plan struct {
	Commands []runner.Command
	Globals  []GlobalStep
	Servers  []mcp.Server
}

// GlobalStep é uma instalação global feita uma vez, identificada por Key.
// O rastreamento de "já instalado" é responsabilidade de uma fase posterior.
type GlobalStep struct {
	Key      string
	Commands []runner.Command
}

// Options carrega decisões que o chamador já resolveu antes de Resolve.
// Global marca componentes `via: skills` como conteúdo **pessoal**
// cross-project (`-g` do `npx skills add`) — não é o caminho normal dos
// componentes de projeto (I1 design §3.3): por padrão, componentes instalam
// project-local e são vendorizados/commitados em `.claude/` (ver
// scaffold.MergeGitignore). `via: aitmpl` não tem noção de global e ignora
// esta flag. VaultPath/UserDocsVaultPath são caminhos já resolvidos; vazio em
// UserDocsVaultPath significa "não configurado".
type Options struct {
	Global            bool
	VaultPath         string
	UserDocsVaultPath string
}

// Resolve monta o Plan a partir de p e opts. Retorna erro imediatamente se um
// componente tiver via desconhecido (defensivo: p já deveria ter passado por Validate).
func Resolve(p *profile.Profile, opts Options) (Plan, error) {
	var plan Plan
	for _, c := range p.Components {
		cmd, err := componentCommand(c, opts)
		if err != nil {
			return Plan{}, err
		}
		plan.Commands = append(plan.Commands, cmd)
	}
	resolveIntegrations(p.Integrations, opts, &plan)
	return plan, nil
}

// componentCommand mapeia um Component no comando npx que o instala.
// Project-local por padrão (I1): só `via: skills` com opts.Global adiciona
// `-g`, marcando o componente como conteúdo pessoal cross-project em vez de
// vendorizado no projeto.
func componentCommand(c profile.Component, opts Options) (runner.Command, error) {
	switch c.Via {
	case profile.ViaSkills:
		args := []string{"skills", "add", c.Source, "--skill", c.Skill, "-a", "claude-code", "-y"}
		if opts.Global {
			args = append(args, "-g")
		}
		return runner.Command{Name: "npx", Args: args}, nil
	case profile.ViaAitmpl:
		flag, err := aitmplFlag(c.Type)
		if err != nil {
			return runner.Command{}, err
		}
		return runner.Command{
			Name: "npx",
			Args: []string{"claude-code-templates@latest", flag + "=" + c.Ref, "--yes"},
		}, nil
	default:
		return runner.Command{}, fmt.Errorf("unknown via %q", c.Via)
	}
}

// aitmplFlag traduz o Type de um componente aitmpl na flag do claude-code-templates.
func aitmplFlag(t string) (string, error) {
	switch t {
	case profile.TypeAgent:
		return "--agent", nil
	case profile.TypeCommand:
		return "--command", nil
	case profile.TypeMCP:
		return "--mcp", nil
	default:
		return "", fmt.Errorf("unknown aitmpl type %q", t)
	}
}
