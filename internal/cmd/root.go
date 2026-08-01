// Package cmd is the mount point of ray's command tree.
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/TheBud4/ray/internal/preflight"
	"github.com/TheBud4/ray/internal/runner"
)

// Global flags, shared between subcommands.
var (
	flagVerbose bool
	flagDryRun  bool
)

func newRootCmd() *cobra.Command {

	root := &cobra.Command{
		Use:   "ray",
		Short: "Personal CLI for bootstrapping projects and AI dev environments",
		// O Cobra não dá `--version` de graça: só registra a flag quando este
		// campo está preenchido. Sem ele, `ray --version` respondia "unknown
		// flag" e não havia como saber qual build estava rodando.
		Version:       buildVersion(),
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Sempre o diretório corrente: `ray` sem subcomando não aceita
			// caminho — quem quer diagnosticar outro projeto usa
			// `ray status <path>`.
			return runFirstRun(
				preflight.RunnerLooker{Runner: runner.ExecRunner{}},
				".", cmd.OutOrStdout())
		},
	}

	// O `v` é declarado aqui de propósito: o Cobra só dá o atalho a `--version`
	// se o encontrar livre no Execute, então quem chega antes fica com ele.
	// Sem isto, `ray -v <comando>` imprime a versão, não roda o comando e sai
	// 0 — sucesso aparente em script.
	root.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "verbose output")
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
