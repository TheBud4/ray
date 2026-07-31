package status

import (
	"fmt"
	"os"

	"github.com/TheBud4/ray/internal/mcp"
	"github.com/TheBud4/ray/internal/preflight"
)

// checkMCP verifica só o que o ray de fato sabe sobre os servidores.
//
// Não existe checagem genérica de "caminho morto": mcp.Server tem
// Command/Args/Env e nada é tipado como caminho, então varrer os args
// procurando o que "parece caminho" seria adivinhação — e o .mcp.json também
// guarda servidores que o usuário acrescentou à mão. Sobram duas perguntas
// verificáveis sem heurística: o RAY_BRAIN, que é do ray, e o Command, que ou
// está no PATH ou não está.
//
// l é um preflight.PathLooker: a pergunta se responde com lookup, e rodar
// `<command> --version` de cada entrada faria um comando de diagnóstico
// executar binário de terceiro só para saber que ele existe.
func checkMCP(l preflight.Looker, target string) (int, []string, error) {
	servers, err := mcp.ReadServers(target)
	if err != nil {
		return 0, nil, err
	}

	var problems []string
	if brain := os.Getenv("RAY_BRAIN"); brain != "" {
		if _, err := os.Stat(brain); err != nil {
			problems = append(problems, fmt.Sprintf("RAY_BRAIN points at %s, which does not exist", brain))
		}
	}
	for _, s := range servers {
		if s.Command == "" || l == nil {
			continue
		}
		if !l.Look(s.Command) {
			problems = append(problems, fmt.Sprintf("mcp/%s: command %q is not on PATH", s.Name, s.Command))
		}
	}
	return len(servers), problems, nil
}
