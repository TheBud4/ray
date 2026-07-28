package installer

import (
	"github.com/TheBud4/ray/internal/economy"
	"github.com/TheBud4/ray/internal/mcp"
	"github.com/TheBud4/ray/internal/profile"
)

// resolveIntegrations acrescenta ao plan as entradas ditadas pelas integrações
// ligadas na receita (tabela §6 do guia). Headroom/CodeGraph são mecanismos de
// Token Economy (I4, design §8.1) — a implementação concreta vive em
// internal/economy; aqui só se decide *se* cada um entra, preservando a
// posição de cada branch (plan.Servers é comparado por ordem nos testes).
func resolveIntegrations(in profile.Integrations, opts Options, plan *Plan) {
	if in.Headroom {
		applyMechanism(plan, economy.Headroom())
	}
	if in.Brain {
		addBrain(opts, plan)
	}
	if in.CodeGraph {
		applyMechanism(plan, economy.CodeGraph())
	}
}

// applyMechanism traduz um economy.Mechanism nas entradas de Plan
// correspondentes: Install vira um GlobalStep (install-once, chave =
// m.Name); Commands (per-projeto, sempre roda) e Server (se houver) são
// acrescentados diretamente.
func applyMechanism(plan *Plan, m economy.Mechanism) {
	if len(m.Install) > 0 {
		plan.Globals = append(plan.Globals, GlobalStep{Key: m.Name, Commands: m.Install})
	}
	plan.Commands = append(plan.Commands, m.Commands...)
	if m.Server != nil {
		plan.Servers = append(plan.Servers, *m.Server)
	}
}

// addBrain expõe o cérebro do usuário por MCP filesystem. Condicional: sem
// path configurado não há server — o aviso é responsabilidade do initai.
func addBrain(opts Options, plan *Plan) {
	if opts.BrainPath == "" {
		return
	}
	plan.Servers = append(plan.Servers, filesystemServer("brain", opts.BrainPath))
}

// filesystemServer monta um server @modelcontextprotocol/server-filesystem apontado a path.
func filesystemServer(name, path string) mcp.Server {
	return mcp.Server{
		Name:    name,
		Command: "npx",
		Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", path},
	}
}
