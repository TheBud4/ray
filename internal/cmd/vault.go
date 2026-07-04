package cmd

import "github.com/spf13/cobra"

func newVaultCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "vault",
		Short: "Create new vault",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
}
