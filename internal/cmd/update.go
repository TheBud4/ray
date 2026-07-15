package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/TheBud4/ray/internal/raypaths"
	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/update"
)

var (
	flagUpdateProfile string
	flagUpdateForce   bool
)

func newUpdateCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "update [path]",
		Short: "Upgrade tools and re-acquire content, protecting local edits by content hash",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "."
			if len(args) == 1 {
				target = args[0]
			}

			home, err := resolveUpdateHome()
			if err != nil {
				return err
			}

			checkRunner := runner.ExecRunner{}
			execRunner := runner.ExecRunner{DryRun: flagDryRun, Out: cmd.OutOrStdout()}
			opts := update.Options{
				Profile: flagUpdateProfile,
				Target:  target,
				Force:   flagUpdateForce,
				DryRun:  flagDryRun,
				Out:     cmd.OutOrStdout(),
			}

			return runUpdate(execRunner, checkRunner, opts, home, cmd.OutOrStdout())
		},
	}
	c.Flags().StringVar(&flagUpdateProfile, "profile", "", "override the recipe recorded in .claude/.ray-profile")
	c.Flags().BoolVar(&flagUpdateForce, "force", false, "overwrite local edits and proceed despite an uncommitted tree")
	return c
}

// resolveUpdateHome resolve os caminhos de ~/.ray usados por update.Run.
func resolveUpdateHome() (update.Home, error) {
	profilesDir, err := raypaths.ProfilesDir()
	if err != nil {
		return update.Home{}, err
	}
	storeDir, err := raypaths.StoreDir()
	if err != nil {
		return update.Home{}, err
	}
	return update.Home{ProfilesDir: profilesDir, StoreDir: storeDir}, nil
}

// runUpdate é a lógica injetável do comando: roda update.Run, imprime o
// resumo e devolve erro (exit ≠ 0) se algum passo falhou.
func runUpdate(r runner.Runner, check runner.Runner, opts update.Options, home update.Home, out io.Writer) error {
	sum, err := update.Run(r, check, opts, home)
	if err != nil {
		return err
	}
	printUpdateSummary(out, sum)
	if sum.HadFailure {
		return fmt.Errorf("one or more steps failed; see summary above")
	}
	return nil
}

func printUpdateSummary(out io.Writer, sum update.Summary) {
	printList := func(label string, items []string) {
		if len(items) == 0 {
			return
		}
		fmt.Fprintf(out, "%s:\n", label)
		for _, it := range items {
			fmt.Fprintf(out, "  - %s\n", it)
		}
	}
	printList("Updated", sum.Updated)
	printList("Skipped", sum.Skipped)
	printList("Failed", sum.Failed)
	printList("Warnings", sum.Warnings)
}
