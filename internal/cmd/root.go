// Package cmd is the mount point of ray's command tree.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Global flags, shared between subcommands.
var (
	flagVerbose bool
	flagDryRun  bool
)

func newRootCmd() *cobra.Command {

	root := &cobra.Command{
		Use:           "ray",
		Short:         "Personal CLI for bootstrapping projects and AI dev environments",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().BoolVar(&flagVerbose, "verbose", false, "verbose output")
	root.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "print actions without doing them")

	// Build the command tree
	root.AddCommand(newInitCmd(), newNewCmd(), newRunCmd(), newProfileCmd(),
		newBrainCmd(), newDoctorCmd(), newUpdateCmd(), newStatsCmd(), newLearnCmd(),
		newStatusCmd())

	return root

}

func Execute() {
	if err := newRootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
