package cmd

import "github.com/spf13/cobra"

func newInitCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "init", Short: "Initialization Commands"}
	cmd.AddCommand(newInitAICmd())
	return cmd
}

func newInitAICmd() *cobra.Command {
	return &cobra.Command{
		Use:   "ai",
		Short: "Set up the AI development environment in a folder",
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
}
