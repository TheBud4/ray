package cmd

import "github.com/spf13/cobra"

func newInitCmd() *cobra.Command {
	cmd := groupCmd("init", "Initialization Commands")
	cmd.AddCommand(newInitAICmd())
	return cmd
}
