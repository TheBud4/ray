// Package installer traduz as integrações de uma receita validada
// (*profile.Profile) num Plan: dados puros (comandos por-projeto, globais,
// servers) que uma fase posterior executa. Não toca rede, não executa
// processos, não sabe de CLI. Componentes de conteúdo (`p.Components`) não
// passam por aqui — são cópia local pura, resolvida direto em
// internal/initai/internal/update; este pacote só resolve o que a tabela §6
// do build guide chama de integrações (headroom, brain, code_graph).
package installer

import (
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
// BrainPath é um caminho já resolvido; vazio significa "não configurado".
type Options struct {
	BrainPath string
}

// Resolve monta o Plan a partir das integrações de p. p.Components nunca
// entra aqui — é cópia local pura, resolvida direto em internal/initai e
// internal/update.
func Resolve(p *profile.Profile, opts Options) (Plan, error) {
	var plan Plan
	resolveIntegrations(p.Integrations, opts, &plan)
	return plan, nil
}
