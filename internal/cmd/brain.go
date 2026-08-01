package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/TheBud4/ray/internal/openutil"
	"github.com/TheBud4/ray/internal/rayconfig"
	"github.com/TheBud4/ray/internal/raypaths"
	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/vault"
)

// newBrainCmd substitui os antigos grupos `ray vault` e `ray docs`. Os dois
// apontavam servers MCP idênticos para diretórios do mesmo tipo; a distinção
// entre "vault da IA" e "vault do usuário" só existia na prosa das regras.
func newBrainCmd() *cobra.Command {
	c := groupCmd("brain", "Manage the brain: the Obsidian vault the AI reads and writes")
	c.AddCommand(newBrainSetCmd(), newBrainStatusCmd(), newBrainOpenCmd(), newBrainPathCmd())
	return c
}

func newBrainSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <path>",
		Short: "Point ray at an existing vault (never creates or reorganizes it)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := raypaths.ConfigPath()
			if err != nil {
				return err
			}
			return runBrainSet(configPath, args[0], cmd.OutOrStdout())
		},
	}
}

// runBrainSet valida o caminho e o grava. Confirma nomeando o que gravou:
// gravar configuração em silêncio obriga quem rodou a checar com um segundo
// comando se pegou — e o caminho impresso é o que o `vault.Verify` aceitou,
// não o que foi digitado.
func runBrainSet(configPath, path string, out io.Writer) error {
	if err := vault.Verify(path); err != nil {
		return err
	}
	cfg, err := rayconfig.Load(configPath)
	if err != nil {
		return err
	}
	if err := cfg.SetBrain(configPath, path); err != nil {
		return err
	}
	fmt.Fprintf(out, "brain set to %s\n", path)
	return nil
}

func newBrainStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the brain's path, existence and note count",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := raypaths.ConfigPath()
			if err != nil {
				return err
			}
			return runBrainStatus(configPath, cmd.OutOrStdout())
		},
	}
}

func runBrainStatus(configPath string, out io.Writer) error {
	cfg, err := rayconfig.Load(configPath)
	if err != nil {
		return err
	}
	path := cfg.BrainPath()
	if path == "" {
		fmt.Fprintln(out, "brain: not configured (run `ray brain set <path>`)")
		return nil
	}
	st, err := vault.Stat(path)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "path: %s\n", st.Path)
	fmt.Fprintf(out, "exists: %s\n", yesNo(st.Exists))
	fmt.Fprintf(out, "markdown files: %d\n", st.MarkdownCount)
	fmt.Fprintln(out, "reminder: the brain MCP server points here when the brain integration is enabled.")
	return nil
}

func newBrainOpenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "open",
		Short: "Open the brain in the default app",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := raypaths.ConfigPath()
			if err != nil {
				return err
			}
			return runBrainOpen(runner.ExecRunner{}, configPath)
		},
	}
}

func runBrainOpen(r runner.Runner, configPath string) error {
	path, err := resolveBrainPath(configPath)
	if err != nil {
		return err
	}
	return openutil.Open(r, path)
}

func newBrainPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the configured brain directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := raypaths.ConfigPath()
			if err != nil {
				return err
			}
			path, err := resolveBrainPath(configPath)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		},
	}
}

func resolveBrainPath(configPath string) (string, error) {
	cfg, err := rayconfig.Load(configPath)
	if err != nil {
		return "", err
	}
	path := cfg.BrainPath()
	if path == "" {
		return "", fmt.Errorf("brain not configured; run `ray brain set <path>`")
	}
	return path, nil
}
