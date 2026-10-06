package cmd

import (
	"fmt"
	"io"

	"github.com/TheBud4/ray/internal/status"
)

// runFirstRun imprime a tela de `ray` sem subcomando. Ela orienta, não
// diagnostica: o `ray status` responde "como está este projeto" melhor do que
// uma tela de abertura conseguiria, e por isso aqui não há git, não há veredito
// sobre cada servidor MCP e não há receita carregada. A linha de fatos é a
// mesma do `ray status`, inventário incluído — duas contagens diferentes para
// o mesmo projeto seria pior que não contar.
//
// Não checa dependência: sem receita carregada nada é obrigatório, então não
// haveria o que avisar, e criar um processo por ferramenta na tela mais vista
// do CLI seria custo sem retorno. Quem diagnostica dependência é o `ray
// doctor`.
//
// Devolve erro só em falha de leitura.
func runFirstRun(target string, out io.Writer) error {
	facts, err := status.ReadFacts(target)
	if err != nil {
		return err
	}

	fmt.Fprintln(out, "ray — versioned AI environments")

	if facts.HasEnvironment {
		fmt.Fprintln(out)
		printFacts(out, facts.Profile, facts.Inventory)
		fmt.Fprintln(out, "\nNext steps:")
		fmt.Fprintln(out, "  claude                start the session")
		fmt.Fprintln(out, "  ray status            diagnose the environment")
	} else {
		fmt.Fprintln(out, "\nNext steps:")
		fmt.Fprintln(out, "  ray new go my-app     new project, environment included")
		fmt.Fprintln(out, "  ray init ai           environment only, in this directory")
	}

	fmt.Fprintln(out, "\n`ray --help` lists every command")
	return nil
}
