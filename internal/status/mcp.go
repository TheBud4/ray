package status

import (
	"fmt"
	"os"

	"github.com/TheBud4/ray/internal/mcp"
	"github.com/TheBud4/ray/internal/preflight"
	"github.com/TheBud4/ray/internal/rayconfig"
)

// checkMCP verifica só o que o ray de fato sabe sobre os servidores.
//
// Não existe checagem genérica de "caminho morto": mcp.Server tem
// Command/Args/Env e nada é tipado como caminho, então varrer os args
// procurando o que "parece caminho" seria adivinhação — e o .mcp.json também
// guarda servidores que o usuário acrescentou à mão. Sobram duas perguntas
// verificáveis sem heurística: o caminho do cérebro, que é do ray (o gravado em
// config.yaml ou o override RAY_BRAIN), e o Command, que ou está no PATH ou
// não está.
//
// l é um preflight.PathLooker: a pergunta se responde com lookup, e rodar
// `<command> --version` de cada entrada faria um comando de diagnóstico
// executar binário de terceiro só para saber que ele existe.
func checkMCP(l preflight.Looker, target, configPath string) ([]string, error) {
	servers, err := mcp.ReadServers(target)
	if err != nil {
		return nil, err
	}

	var problems []string
	if p := checkBrainPath(configPath); p != "" {
		problems = append(problems, p)
	}
	for _, s := range servers {
		if s.Command == "" || l == nil {
			continue
		}
		if !l.Look(s.Command) {
			problems = append(problems, fmt.Sprintf("mcp/%s: command %q is not on PATH", s.Name, s.Command))
		}
	}
	return problems, nil
}

// checkBrainPath devolve o problema do cérebro efetivo (RAY_BRAIN, senão o do
// config.yaml), ou vazio. Sem cérebro configurado não há o que reportar: o
// cérebro é opcional, e quem o configurou é quem `ray brain status` atende.
func checkBrainPath(configPath string) string {
	if env := os.Getenv("RAY_BRAIN"); env != "" {
		if _, err := os.Stat(env); err != nil {
			return fmt.Sprintf("RAY_BRAIN points at %s, which does not exist", env)
		}
		return ""
	}
	if configPath == "" {
		return ""
	}
	cfg, err := rayconfig.Load(configPath)
	if err != nil {
		return fmt.Sprintf("config.yaml is unreadable: %v", err)
	}
	if cfg.Brain == "" {
		return ""
	}
	if _, err := os.Stat(cfg.Brain); err != nil {
		return fmt.Sprintf("the brain in config.yaml points at %s, which does not exist; run `ray brain set <path>`", cfg.Brain)
	}
	return ""
}
