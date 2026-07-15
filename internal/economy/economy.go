// Package economy modela "Token Economy" (design §8.1–§8.2): os mecanismos
// que economizam tokens de IA — grafo de código, compressão de contexto, e
// handoff entre sessões — como implementações plugáveis de um contrato
// comum, em vez de flags soltas espalhadas pelo installer. Prepara o terreno
// para `ray stats` (I5) ler MetricKey por mecanismo.
package economy

import (
	"github.com/TheBud4/ray/internal/mcp"
	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/runner"
)

// Mechanism é o contrato comum a todo mecanismo de economia de tokens.
// Name é a identidade estável do mecanismo — dobra como GlobalStep.Key
// (install-once, persistido em ~/.ray/state.yaml) para os de Kind "mcp", por
// isso usa as chaves legadas ("headroom", "code_graph"), não os slugs
// ilustrativos do design doc. Install é a instalação global, uma vez;
// Commands é comando por-projeto, sempre roda (ex. reindexar o grafo);
// Server, se o mecanismo expõe um MCP server. Um mecanismo builtin/hook
// (handoff) deixa Install/Commands/Server vazios — já é modelado inteiramente
// via scaffold (internal/scaffold/mode.go).
type Mechanism struct {
	Name      string
	Kind      string // "mcp" | "hook"
	Install   []runner.Command
	Commands  []runner.Command
	Server    *mcp.Server
	MetricKey string
}

// Headroom é o mecanismo de compressão de contexto (design §8.1).
func Headroom() Mechanism {
	return Mechanism{
		Name:      "headroom",
		Kind:      "mcp",
		Install:   []runner.Command{{Name: "uv", Args: []string{"tool", "install", "headroom-ai[mcp]"}}},
		Server:    &mcp.Server{Name: "headroom", Command: "headroom", Args: []string{"mcp"}},
		MetricKey: "compressions",
	}
}

// CodeGraph é o mecanismo de grafo de código (design §8.1): a IA consulta o
// grafo via MCP em vez de reabrir arquivos.
func CodeGraph() Mechanism {
	return Mechanism{
		Name: "code_graph",
		Kind: "mcp",
		Install: []runner.Command{
			{Name: "uv", Args: []string{"tool", "install", "graphifyy"}},
			{Name: "graphify", Args: []string{"install", "--platform", "claude"}},
		},
		Commands:  []runner.Command{{Name: "graphify", Args: []string{"update", "."}}},
		Server:    &mcp.Server{Name: "graphify", Command: "graphify-mcp"},
		MetricKey: "graph_queries",
	}
}

// Handoff é o mecanismo de continuidade entre sessões (design §8.1): sempre
// presente, implementado inteiramente como scaffold (hook SessionStart +
// .claude/handoff.md) — nenhuma instalação nem MCP server.
func Handoff() Mechanism {
	return Mechanism{
		Name:      "handoff",
		Kind:      "hook",
		MetricKey: "handoffs",
	}
}

// Mechanisms devolve os mecanismos ativos para in: Handoff() sempre (é
// built-in, não uma integração ligável), mais CodeGraph()/Headroom() se a
// receita os liga.
func Mechanisms(in profile.Integrations) []Mechanism {
	mechs := []Mechanism{Handoff()}
	if in.CodeGraph {
		mechs = append(mechs, CodeGraph())
	}
	if in.Headroom {
		mechs = append(mechs, Headroom())
	}
	return mechs
}
