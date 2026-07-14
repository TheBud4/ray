package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/TheBud4/ray/internal/openutil"
	"github.com/TheBud4/ray/internal/raypaths"
	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/vault"
)

func newVaultCmd() *cobra.Command {
	c := &cobra.Command{Use: "vault", Short: "Manage the AI knowledge vault (~/.ray/vault)"}
	c.AddCommand(newVaultInitCmd(), newVaultStatusCmd(), newVaultOpenCmd(), newVaultPathCmd())
	return c
}

func newVaultInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init",
		Short: "Create the vault layout if missing",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := raypaths.VaultDir()
			if err != nil {
				return err
			}
			return vault.Ensure(dir)
		},
	}
}

func newVaultStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the vault's path, existence and note count",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := raypaths.VaultDir()
			if err != nil {
				return err
			}
			return runVaultStatus(dir, cmd.OutOrStdout())
		},
	}
}

func runVaultStatus(dir string, out io.Writer) error {
	st, err := vault.Stat(dir)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "path: %s\n", st.Path)
	fmt.Fprintf(out, "exists: %s\n", yesNo(st.Exists))
	fmt.Fprintf(out, "markdown files: %d\n", st.MarkdownCount)
	fmt.Fprintln(out, "reminder: the vault-fs MCP server points here when knowledge_vault is enabled.")
	return nil
}

func newVaultOpenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "open",
		Short: "Open the vault in the default app",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := raypaths.VaultDir()
			if err != nil {
				return err
			}
			return runVaultOpen(runner.ExecRunner{}, dir)
		},
	}
}

func runVaultOpen(r runner.Runner, dir string) error {
	return openutil.Open(r, dir)
}

func newVaultPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the vault directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := raypaths.VaultDir()
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), dir)
			return nil
		},
	}
}
