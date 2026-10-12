// Package cmd is the mount point of ray's command tree.
package cmd

import (
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/TheBud4/ray/internal/ui"
)

// Global flags, shared between subcommands.
var (
	flagVerbose bool
	flagDryRun  bool
)

// groupCmd é o esqueleto de um comando que só agrupa filhos: sem argumento
// imprime o help, com argumento recusa.
//
// O `Args` sozinho não bastaria. O Cobra checa `Runnable()` antes de validar
// argumento, então um grupo sem `RunE` devolve o help e `nil` para qualquer
// filho inexistente — `ray profile lst` saía 0 sem ter feito nada. É o `RunE`
// que torna o comando runnable e faz o `NoArgs` ser consultado.
func groupCmd(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  cobra.NoArgs,
		RunE:  func(cmd *cobra.Command, args []string) error { return cmd.Help() },
	}
}

func newRootCmd() *cobra.Command { return newRootCmdWith(os.Getenv) }

// newRootCmdWith monta a árvore lendo o ambiente por getenv, para a cor ser
// decidível em teste sem tocar o ambiente do processo.
func newRootCmdWith(getenv func(string) string) *cobra.Command {
	var color ui.ModeValue

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
		// Roda depois do parse das flags e da validação dos argumentos: erro de
		// flag, de argumento ou de subcomando desconhecido sai antes daqui, e a
		// linha de erro (run) não depende deste embrulho. Nenhum subcomando pode
		// declarar o próprio PersistentPreRun(E): ele desligaria este em silêncio.
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			r := cmd.Root()
			if w := ui.Stream(r.OutOrStdout(), os.Stdout, color.Mode, getenv); w != r.OutOrStdout() {
				r.SetOut(w)
			}
			if w := ui.Stream(r.ErrOrStderr(), os.Stderr, color.Mode, getenv); w != r.ErrOrStderr() {
				r.SetErr(w)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			// Sempre o diretório corrente: `ray` sem subcomando não aceita
			// caminho — quem quer diagnosticar outro projeto usa
			// `ray status <path>`.
			return runFirstRun(".", cmd.OutOrStdout())
		},
	}

	// O `v` é declarado aqui de propósito: o Cobra só dá o atalho a `--version`
	// se o encontrar livre no Execute, então quem chega antes fica com ele.
	// Sem isto, `ray -v <comando>` imprime a versão, não roda o comando e sai
	// 0 — sucesso aparente em script.
	root.PersistentFlags().BoolVarP(&flagVerbose, "verbose", "v", false, "print each step before running it (only used by `ray run`)")
	root.PersistentFlags().BoolVar(&flagDryRun, "dry-run", false, "print actions without doing them")
	// Sem NoOptDefVal: `--color always` exige o valor e não vira argumento
	// posicional. Valor fora de auto|always|never é recusado no parse.
	root.PersistentFlags().Var(&color, "color", "colorize the output: auto, always or never")

	// Build the command tree
	root.AddCommand(newInitCmd(), newNewCmd(), newRunCmd(), newProfileCmd(),
		newTemplateCmd(), newBrainCmd(), newDoctorCmd(), newUpdateCmd(), newStatsCmd(),
		newStatusCmd(), newServeCmd())

	return root

}

// Execute roda o ray e sai com o código de run.
func Execute() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, os.Getenv))
}

// run é o Execute testável: devolve o código de saída em vez de sair. A linha
// de erro nasce aqui e decide a cor do stderr sozinha, porque erro de flag, de
// argumento e de subcomando desconhecido saem antes de qualquer PreRun.
func run(args []string, stdout, stderr io.Writer, getenv func(string) string) int {
	root := newRootCmdWith(getenv)
	if stdout != io.Writer(os.Stdout) {
		root.SetOut(stdout)
	}
	if stderr != io.Writer(os.Stderr) {
		root.SetErr(stderr)
	}
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		mode := ui.Auto
		if f := root.PersistentFlags().Lookup("color"); f != nil {
			mode, _ = ui.ParseMode(f.Value.String())
		}
		ui.For(ui.Stream(stderr, os.Stderr, mode, getenv)).Error(err.Error())
		return 1
	}
	return 0
}
