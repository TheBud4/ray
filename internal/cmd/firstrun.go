package cmd

import (
	"fmt"
	"io"

	"github.com/TheBud4/ray/internal/preflight"
)

// runFirstRun imprime a tela de `ray` sem subcomando. Ela orienta, não
// diagnostica: o `ray status` responde "como está este projeto" melhor do que
// uma tela de abertura conseguiria, e por isso aqui não há git, não há MCP e
// não há receita carregada.
//
// Devolve erro só em falha de leitura. Dependência required faltando é alerta
// na tela, não exit ≠ 0 — quem erra por dependência é o doctor.
func runFirstRun(l preflight.Looker, target string, out io.Writer) error {
	fmt.Fprintln(out, "ray — versioned AI environments")
	printMissingRequired(out, l)

	fmt.Fprintln(out, "\nNext steps:")
	fmt.Fprintln(out, "  ray new go my-app     new project, environment included")
	fmt.Fprintln(out, "  ray init ai           environment only, in this directory")
	fmt.Fprintln(out, "\n`ray --help` lists every command")
	return nil
}

// printMissingRequired é a única linha de dependência da tela, e só existe
// quando falta required — silêncio é o normal.
//
// needPython é false: sem receita carregada não há como saber se o perfil usa
// Python, e chutar que usa produziria alerta falso na tela mais vista do CLI.
// O `ray doctor` continua dono da tabela completa.
func printMissingRequired(out io.Writer, l preflight.Looker) {
	for _, c := range preflight.MissingRequired(preflight.Run(l, false)) {
		fmt.Fprintf(out, "\n⚠ missing %s", c.Name)
		if c.Hint != "" {
			fmt.Fprintf(out, " — %s", c.Hint)
		}
		fmt.Fprintln(out)
		fmt.Fprintln(out, "  run `ray doctor` for the full table")
	}
}
