package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/TheBud4/ray/internal/learn"
	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/raypaths"
	"github.com/TheBud4/ray/internal/runner"
)

var flagLearnProfile string

func newLearnCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "learn",
		Short: "Learn-mode verifiable machine (milestones, journal)",
	}
	c.AddCommand(newLearnCheckCmd())
	return c
}

func newLearnCheckCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "check [path]",
		Short: "Run the current milestone's verify command and record its passage in the learning journal",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "."
			if len(args) == 1 {
				target = args[0]
			}
			profilesDir, err := raypaths.ProfilesDir()
			if err != nil {
				return err
			}
			r := runner.ExecRunner{}
			return runLearnCheck(r, profilesDir, target, flagLearnProfile, cmd.OutOrStdout())
		},
	}
	c.Flags().StringVar(&flagLearnProfile, "profile", "", "override the recipe recorded in .claude/.ray-profile")
	return c
}

// runLearnCheck é a lógica injetável do comando: resolve o perfil, roda
// learn.Check para o marco corrente e imprime o desfecho.
func runLearnCheck(r runner.Runner, profilesDir, target, overrideProfile string, out io.Writer) error {
	prof, err := profile.LoadForTarget(profilesDir, target, overrideProfile)
	if err != nil {
		return err
	}

	ms, err := learn.LoadMilestones(target, prof.Milestones)
	if err != nil {
		return err
	}
	res, ok, err := learn.Check(r, target, ms)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(out, "no milestones to check (none defined, or all already passed)")
		return nil
	}
	if res.Passed {
		fmt.Fprintf(out, "✓ milestone passed: %s\n", res.Milestone.Goal)
		return nil
	}
	fmt.Fprintf(out, "✗ milestone not yet passed: %s\n%s", res.Milestone.Goal, res.Output)
	return fmt.Errorf("milestone check failed")
}
