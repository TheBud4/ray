package cmd

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/TheBud4/ray/internal/preflight"
	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/ui"
)

var flagFix bool

func newDoctorCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "doctor",
		Args:  cobra.NoArgs,
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
	p := ui.For(out)

	if fix {
		uvWasMissing := false
		uvInstalled := false
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
				if uvInstalled {
					p.Warning(fmt.Sprintf("skip fix %s: needs uv, which was just installed — reopen your shell and run `ray doctor --fix` again", c.Name), "")
				} else {
					p.Warning(fmt.Sprintf("skip fix %s: needs uv, whose install failed", c.Name), "")
				}
				continue
			}
			fixOK := true
			for _, cmd := range c.Fix {
				res, err := fixRunner.Run(context.Background(), cmd)
				if err != nil || res.ExitCode != 0 {
					fixOK = false
					p.Verdict(ui.Fail, fmt.Sprintf("fix %s failed: %s", c.Name, runner.FailureReason(res, err)), "")
				}
			}
			if c.Name == "uv" {
				uvInstalled = fixOK
			}
		}
		checks = preflight.Run(l, true)
		if uvInstalled {
			p.Warning("uv was just installed — reopen your shell so PATH picks it up.", "")
		}
	}

	printDoctorTable(p, checks)

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

func printDoctorTable(p *ui.Printer, checks []preflight.Check) {
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

	header := []string{"NAME", "FOUND", "REQUIRED"}
	if anyAdvice {
		header = append(header, "HINT")
	}
	rows := make([][]ui.Cell, len(checks))
	for i, c := range checks {
		found := ui.Cell{Text: yesNo(c.Found), Color: ui.Green}
		if !c.Found {
			found.Color = ui.Gray
			if c.Required {
				found.Color = ui.Red
			}
		}
		rows[i] = []ui.Cell{{Text: c.Name}, found, {Text: yesNo(c.Required)}}
		if anyAdvice {
			rows[i] = append(rows[i], ui.Cell{Text: advice[i]})
		}
	}
	p.Table(header, rows)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
