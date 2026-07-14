package cmd

import "github.com/spf13/cobra"

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "init", Short: "Initialization Commands"}
	cmd.AddCommand(newInitAICmd())
	return cmd
}
