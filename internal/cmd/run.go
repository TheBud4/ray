package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/TheBud4/ray/internal/raypaths"
	"github.com/TheBud4/ray/internal/runfile"
	"github.com/TheBud4/ray/internal/runner"
)

var flagRunList bool

func newRunCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "run [alias] [-- extra args]",
		Short: "Run a project or global command alias",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			alias, extra := splitAliasArgs(args, cmd.ArgsLenAtDash())

			workdir, err := os.Getwd()
			if err != nil {
				return err
			}
			globalPath, err := raypaths.CommandsPath()
			if err != nil {
				return err
			}
			commands, err := runfile.Load(workdir, globalPath)
			if err != nil {
				return err
			}

			r := runner.ExecRunner{DryRun: flagDryRun, Out: cmd.OutOrStdout()}
			return runRunCmd(commands, alias, extra, flagRunList, r, flagVerbose, cmd.OutOrStdout())
		},
	}
	c.Flags().BoolVar(&flagRunList, "list", false, "list available aliases")
	return c
}

// splitAliasArgs separa o alias dos args após `--` (dashAt = -1 quando não há
// `--`, vem de cmd.ArgsLenAtDash()).
func splitAliasArgs(args []string, dashAt int) (alias string, extra []string) {
	if dashAt == -1 {
		if len(args) > 0 {
			alias = args[0]
		}
		return alias, nil
	}
	if dashAt > 0 {
		alias = args[0]
	}
	return alias, args[dashAt:]
}

func runRunCmd(commands map[string]runfile.Resolved, alias string, extra []string, list bool, r runner.Runner, verbose bool, out io.Writer) error {
	if list || alias == "" {
		printAliasList(out, commands)
		return nil
	}

	res, ok := commands[alias]
	if !ok {
		return fmt.Errorf("unknown alias %q (see `ray run --list`)", alias)
	}

	for i, step := range res.Steps {
		fields := strings.Fields(step)
		if len(fields) == 0 {
			continue
		}
		if i == len(res.Steps)-1 {
			fields = append(fields, extra...)
		}
		if verbose {
			fmt.Fprintf(out, "> %s\n", step)
		}
		result, err := r.Run(context.Background(), runner.Command{Name: fields[0], Args: fields[1:], Dir: res.BaseDir})
		if err != nil {
			return err
		}
		io.WriteString(out, result.Stdout)
		io.WriteString(out, result.Stderr)
		if result.ExitCode != 0 {
			return fmt.Errorf("step %q exited with code %d", step, result.ExitCode)
		}
	}
	return nil
}

func printAliasList(out io.Writer, commands map[string]runfile.Resolved) {
	// O cabeçalho só faz sentido com linha embaixo: sozinho, ele sugere que
	// algo deveria estar listado. Nomear o arquivo evita a pergunta seguinte.
	if len(commands) == 0 {
		fmt.Fprintln(out, "no runs defined (add a commands: block to ray.yaml)")
		return
	}

	names := make([]string, 0, len(commands))
	for name := range commands {
		names = append(names, name)
	}
	sort.Strings(names)

	w := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tSOURCE\tDESCRIPTION")
	for _, name := range names {
		r := commands[name]
		fmt.Fprintf(w, "%s\t%s\t%s\n", r.Name, r.Source, r.Description)
	}
	w.Flush()
}
