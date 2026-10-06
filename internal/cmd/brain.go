package cmd

import (
	"fmt"
	"io"
	"path/filepath"

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
			if flagDryRun {
				// Só leitura: valida o caminho como o comando real e imprime a
				// mudança planejada, sem tocar no config.yaml.
				abs, err := verifiedAbs(args[0])
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "+ set brain to %s (in %s)\n", abs, configPath)
				return nil
			}
			return runBrainSet(configPath, args[0], cmd.OutOrStdout())
		},
	}
}

// verifiedAbs valida path com vault.Verify e o devolve absoluto: o config.yaml
// é global, e um "." ou caminho relativo gravado nele apontaria para outra
// pasta quando lido de outro diretório.
func verifiedAbs(path string) (string, error) {
	if err := vault.Verify(path); err != nil {
		return "", err
	}
	return filepath.Abs(path)
}

// runBrainSet valida o caminho e o grava. Confirma nomeando o que gravou:
// gravar configuração em silêncio obriga quem rodou a checar com um segundo
// comando se pegou — e o caminho impresso é o que o `vault.Verify` aceitou,
// não o que foi digitado.
func runBrainSet(configPath, path string, out io.Writer) error {
	path, err := verifiedAbs(path)
	if err != nil {
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
		Args:  cobra.NoArgs,
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
	return nil
}

func newBrainOpenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "open",
		Args:  cobra.NoArgs,
		Short: "Open the brain in the default app",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := raypaths.ConfigPath()
			if err != nil {
				return err
			}
			return runBrainOpen(runner.ExecRunner{DryRun: flagDryRun, Out: cmd.OutOrStdout()}, configPath)
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
		Args:  cobra.NoArgs,
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
