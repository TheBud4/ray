package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/TheBud4/ray/internal/openutil"
	"github.com/TheBud4/ray/internal/rayconfig"
	"github.com/TheBud4/ray/internal/raypaths"
	"github.com/TheBud4/ray/internal/runner"
)

const docsReadme = `# Docs

Vault central de documentação do usuário, cross-project. Compatível com Obsidian.

- ` + "`guides/`" + ` — guias e how-tos.
- ` + "`concepts/`" + ` — conceitos e referências duráveis.
`

func newDocsCmd() *cobra.Command {
	c := &cobra.Command{Use: "docs", Short: "Manage the user's central docs vault"}
	c.AddCommand(newDocsInitCmd(), newDocsSetCmd(), newDocsOpenCmd(), newDocsPathCmd())
	return c
}

func newDocsInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "init <path>",
		Short: "Create a new Obsidian-compatible docs vault and configure it",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := raypaths.ConfigPath()
			if err != nil {
				return err
			}
			return runDocsInit(configPath, args[0])
		},
	}
}

func runDocsInit(configPath, path string) error {
	if err := ensureDocsVault(path); err != nil {
		return err
	}
	cfg, err := rayconfig.Load(configPath)
	if err != nil {
		return err
	}
	return cfg.SetUserDocsVault(configPath, path)
}

// ensureDocsVault cria o layout mínimo do vault central do usuário
// (guides/, concepts/, README.md, .obsidian/), idempotente — nunca
// sobrescreve. Não reaproveita internal/vault: domínio conceitualmente
// diferente (vault da IA vs. vault central do usuário, §9.3), com estrutura
// de pastas própria.
func ensureDocsVault(dir string) error {
	for _, sub := range []string{"guides", "concepts", ".obsidian"} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			return err
		}
	}
	readmePath := filepath.Join(dir, "README.md")
	if _, err := os.Stat(readmePath); err == nil {
		return nil
	}
	return os.WriteFile(readmePath, []byte(docsReadme), 0o644)
}

func newDocsSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <path>",
		Short: "Point to an existing docs vault (does not reorganize it)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := raypaths.ConfigPath()
			if err != nil {
				return err
			}
			return runDocsSet(configPath, args[0])
		},
	}
}

func runDocsSet(configPath, path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("docs vault path does not exist: %s", path)
	}
	if !info.IsDir() {
		return fmt.Errorf("docs vault path is not a directory: %s", path)
	}
	cfg, err := rayconfig.Load(configPath)
	if err != nil {
		return err
	}
	return cfg.SetUserDocsVault(configPath, path)
}

func newDocsOpenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "open",
		Short: "Open the docs vault in the default app",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := raypaths.ConfigPath()
			if err != nil {
				return err
			}
			return runDocsOpen(runner.ExecRunner{}, configPath)
		},
	}
}

func runDocsOpen(r runner.Runner, configPath string) error {
	path, err := resolveDocsPath(configPath)
	if err != nil {
		return err
	}
	return openutil.Open(r, path)
}

func newDocsPathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Short: "Print the configured docs vault path",
		RunE: func(cmd *cobra.Command, args []string) error {
			configPath, err := raypaths.ConfigPath()
			if err != nil {
				return err
			}
			path, err := resolveDocsPath(configPath)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), path)
			return nil
		},
	}
}

func resolveDocsPath(configPath string) (string, error) {
	cfg, err := rayconfig.Load(configPath)
	if err != nil {
		return "", err
	}
	path := cfg.UserDocsVaultPath()
	if path == "" {
		return "", fmt.Errorf("user docs vault not configured; run `ray docs init <path>` or `ray docs set <path>`")
	}
	return path, nil
}
