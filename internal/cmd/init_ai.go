package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TheBud4/ray/internal/initai"
	"github.com/TheBud4/ray/internal/preflight"
	"github.com/TheBud4/ray/internal/raypaths"
	"github.com/TheBud4/ray/internal/runner"
)

var (
	flagProfile         string
	flagForce           bool
	flagNoGlobal        bool
	flagReinstallGlobal bool
)

func newInitAICmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "ai [path]",
		Short: "Set up the AI development environment in a folder",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "."
			if len(args) == 1 {
				target = args[0]
			}

			home, err := resolveInitAIHome()
			if err != nil {
				return err
			}

			checkRunner := runner.ExecRunner{}
			looker := preflight.RunnerLooker{Runner: checkRunner}
			execRunner := runner.ExecRunner{DryRun: flagDryRun, Out: cmd.OutOrStdout()}
			opts := buildInitAIOptions(target, cmd.OutOrStdout())

			return runInitAI(execRunner, looker, opts, home, cmd.OutOrStdout())
		},
	}
	c.Flags().StringVar(&flagProfile, "profile", "", "recipe to install (required)")
	c.Flags().BoolVar(&flagForce, "force", false, "regenerate scaffold files that already exist (never touches .claude/handoff.md)")
	c.Flags().BoolVar(&flagNoGlobal, "no-global", false, "skip all install-once global steps")
	c.Flags().BoolVar(&flagReinstallGlobal, "reinstall-global", false, "ignore state.yaml and reinstall global steps")
	_ = c.MarkFlagRequired("profile")
	return c
}

// buildInitAIOptions traduz os flags do comando em initai.Options.
func buildInitAIOptions(target string, out io.Writer) initai.Options {
	return initai.Options{
		Profile:         flagProfile,
		Target:          target,
		Force:           flagForce,
		NoGlobal:        flagNoGlobal,
		ReinstallGlobal: flagReinstallGlobal,
		DryRun:          flagDryRun,
		Out:             out,
	}
}

// resolveInitAIHome resolve os caminhos de ~/.ray usados por initai.Run.
func resolveInitAIHome() (initai.Home, error) {
	profilesDir, err := raypaths.ProfilesDir()
	if err != nil {
		return initai.Home{}, err
	}
	templatesDir, err := raypaths.TemplatesDir()
	if err != nil {
		return initai.Home{}, err
	}
	configPath, err := raypaths.ConfigPath()
	if err != nil {
		return initai.Home{}, err
	}
	statePath, err := raypaths.StatePath()
	if err != nil {
		return initai.Home{}, err
	}
	storeDir, err := raypaths.StoreDir()
	if err != nil {
		return initai.Home{}, err
	}
	componentsDir, err := raypaths.ComponentsDir()
	if err != nil {
		return initai.Home{}, err
	}
	return initai.Home{
		ProfilesDir:   profilesDir,
		TemplatesDir:  templatesDir,
		ConfigPath:    configPath,
		StatePath:     statePath,
		StoreDir:      storeDir,
		ComponentsDir: componentsDir,
	}, nil
}

// runInitAI é a lógica injetável do comando: roda initai.Run, imprime o
// resumo e devolve erro (exit ≠ 0) se algum passo falhou.
func runInitAI(r runner.Runner, l preflight.Looker, opts initai.Options, home initai.Home, out io.Writer) error {
	sum, err := initai.Run(r, l, opts, home)
	if err != nil {
		return err
	}
	printInitAISummary(out, sum)
	if sum.HadFailure {
		return fmt.Errorf("one or more steps failed; see summary above")
	}
	return nil
}

func printInitAISummary(out io.Writer, sum initai.Summary) {
	printList := func(label string, items []string) {
		if len(items) == 0 {
			return
		}
		fmt.Fprintf(out, "%s:\n", label)
		for _, it := range items {
			fmt.Fprintf(out, "  - %s\n", it)
		}
	}
	printList("Installed", sum.Installed)
	printList("Failed", sum.Failed)
	printList("Created", sum.Created)
	printList("Skipped", sum.Skipped)
	printList("Warnings", sum.Warnings)
	printNextSteps(out, sum)
}

// printNextSteps fecha o loop do vendoring (I1): o `ray` escreve o ambiente no
// repositório do usuário, e sem esta instrução ninguém o commita — um .claude/
// no disco de uma pessoa só não é ambiente reproduzível.
//
// Não aparece quando algum passo falhou: mandar commitar um ambiente escrito
// pela metade grava o estado quebrado.
func printNextSteps(out io.Writer, sum initai.Summary) {
	if sum.HadFailure {
		return
	}
	fmt.Fprintln(out, "\nNext steps:")
	if sum.InGitRepo && len(sum.VersionedPaths) > 0 {
		// Caminhos por nome, nunca `git add -A`/`.`: o guard-add.sh que este
		// mesmo comando acabou de instalar avisa contra add cego.
		fmt.Fprintf(out, "  git add %s\n", strings.Join(sum.VersionedPaths, " "))
		fmt.Fprintln(out, `  git commit -m "chore: vendor ai environment"`)
	}
	fmt.Fprintln(out, "  claude")
}
