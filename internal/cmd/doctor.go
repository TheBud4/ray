package cmd

import (
	"context"
	"fmt"
	"io"
	"strings"
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
	names := make([]string, len(missing))
	for i, c := range missing {
		names[i] = c.Name
	}
	hint := "run `ray doctor --fix`"
	if fix {
		hint = "these have no automatic fix; install them manually"
	}
	return fmt.Errorf("missing required dependencies: %s (%s)", strings.Join(names, ", "), hint)
}

func printDoctorTable(out io.Writer, checks []preflight.Check) {
	w := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tFOUND\tREQUIRED")
	for _, c := range checks {
		fmt.Fprintf(w, "%s\t%s\t%s\n", c.Name, yesNo(c.Found), yesNo(c.Required))
	}
	w.Flush()
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}
