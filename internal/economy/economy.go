// Package economy modela "Token Economy" (design §8.1–§8.2): os mecanismos
// instalados que economizam tokens de IA — grafo de código e compressão de
// contexto — como implementações plugáveis de um contrato comum, em vez de
// flags soltas espalhadas pelo installer. O handoff entre sessões não está
// aqui: é só scaffold (hook SessionStart + .claude/handoff.md, em
// internal/scaffold), sem nada a instalar.
package economy

import (
	"github.com/TheBud4/ray/internal/mcp"
	"github.com/TheBud4/ray/internal/runner"
)

// Mechanism é o contrato comum a todo mecanismo de economia de tokens.
// Name é a identidade estável do mecanismo — dobra como GlobalStep.Key
// (install-once, persistido em ~/.ray/state.yaml) para os de Kind "mcp", por
// isso usa as chaves legadas ("headroom", "code_graph"), não os slugs
// ilustrativos do design doc. Install é a instalação global, uma vez;
// Commands é comando por-projeto, sempre roda (ex. reindexar o grafo);
// Server, se o mecanismo expõe um MCP server.
type Mechanism struct {
	Name     string
	Kind     string // "mcp"
	Install  []runner.Command
	Commands []runner.Command
	Server   *mcp.Server
}

// Headroom é o mecanismo de compressão de contexto (design §8.1).
func Headroom() Mechanism {
	return Mechanism{
		Name:    "headroom",
		Kind:    "mcp",
		Install: []runner.Command{{Name: "uv", Args: []string{"tool", "install", "headroom-ai[mcp]"}}},
		Server:  &mcp.Server{Name: "headroom", Command: "headroom", Args: []string{"mcp"}},
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
		Commands: []runner.Command{{Name: "graphify", Args: []string{"update", "."}}},
		Server:   &mcp.Server{Name: "graphify", Command: "graphify-mcp"},
	}
}
