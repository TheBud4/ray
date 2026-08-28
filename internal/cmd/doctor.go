package cmd

import (
	"context"
	"fmt"
	"io"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/TheBud4/ray/internal/preflight"
	"github.com/TheBud4/ray/internal/runner"
)

var flagFix bool

func newDoctorCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "doctor",
		Short: "Check external dependencies (npx, python, uv, headroom, graphify)",
		RunE: func(cmd *cobra.Command, args []string) error {
			checkRunner := runner.ExecRunner{}
			looker := preflight.RunnerLooker{Runner: checkRunner}
			fixRunner := runner.ExecRunner{DryRun: flagDryRun, Out: cmd.OutOrStdout()}
			return runDoctor(looker, fixRunner, flagFix, cmd.OutOrStdout())
		},
	}
	c.Flags().BoolVar(&flagFix, "fix", false, "install missing dependencies ray knows how to fix")
	return c
}

// runDoctor contém a lógica do comando, injetável para testes: l checa o
// ambiente (sempre real na fiação — checagem não deve respeitar --dry-run,
// senão o diagnóstico mentiria); fixRunner executa os Fix de --fix (esse sim
// respeita --dry-run, pois instala coisas de verdade).
func runDoctor(l preflight.Looker, fixRunner runner.Runner, fix bool, out io.Writer) error {
	checks := preflight.Run(l, true)

	if fix {
		uvWasMissing := false
		for _, c := range checks {
			if c.Found || len(c.Fix) == 0 {
				continue
			}
			if c.Name == "uv" {
				uvWasMissing = true
			} else if uvWasMissing && fixNeedsUV(c.Fix) {
				// Rodar agora falharia: o script que acabou de instalar o uv
				// roda num subshell e não muda o PATH deste processo Go —
				// exec.CommandContext resolveria "uv" contra o PATH herdado
				// no início do processo e não acharia o binário recém-posto
				// em disco.
				fmt.Fprintf(out, "⚠ skip fix %s: needs uv, which was just installed — reopen your shell and run `ray doctor --fix` again\n", c.Name)
				continue
			}
			for _, cmd := range c.Fix {
				if _, err := fixRunner.Run(context.Background(), cmd); err != nil {
					fmt.Fprintf(out, "✗ fix %s failed: %v\n", c.Name, err)
				}
			}
		}
		checks = preflight.Run(l, true)
		if uvWasMissing {
			fmt.Fprintln(out, "uv was just installed — reopen your shell so PATH picks it up.")
		}
	}

	printDoctorTable(out, checks)

	missing := preflight.MissingRequired(checks)
	if len(missing) == 0 {
		return nil
	}
	from := preflight.FromDoctor
	if fix {
		from = preflight.FromDoctorFix
	}
	return &preflight.MissingRequiredError{Missing: missing, From: from}
}

// fixNeedsUV diz se o Fix de uma checagem invoca o binário uv — é o caso de
// headroom e graphify, cujo Fix é `uv tool install ...`.
func fixNeedsUV(fix []runner.Command) bool {
	for _, cmd := range fix {
		if cmd.Name == "uv" {
			return true
		}
	}
	return false
}

func printDoctorTable(out io.Writer, checks []preflight.Check) {
	// Conselho só para o que falta: dizer o que fazer com algo presente é
	// ruído na coluna que existe para ser lida quando algo dá errado. E sem
	// nada a aconselhar a coluna nem aparece — mesma regra que tirou a linha
	// "deps: ok" da tela de abertura.
	advice := make([]string, len(checks))
	anyAdvice := false
	for i, c := range checks {
		if c.Found {
			continue
		}
		advice[i] = preflight.Advice(c, preflight.FromDoctor)
		anyAdvice = anyAdvice || advice[i] != ""
	}

	w := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	header := "NAME\tFOUND\tREQUIRED"
	if anyAdvice {
		header += "\tHINT"
	}
	fmt.Fprintln(w, header)
	for i, c := range checks {
		fmt.Fprintf(w, "%s\t%s\t%s", c.Name, yesNo(c.Found), yesNo(c.Required))
		if anyAdvice {
			fmt.Fprintf(w, "\t%s", advice[i])
		}
		fmt.Fprintln(w)
	}
	w.Flush()
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
