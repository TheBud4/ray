package cmd

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/TheBud4/ray/internal/initai"
	"github.com/TheBud4/ray/internal/preflight"
	"github.com/TheBud4/ray/internal/raypaths"
	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/scaffold"
)

var (
	flagProfile         string
	flagMode            string
	flagGlobal          bool
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
	c.Flags().StringVar(&flagMode, "mode", scaffold.ModeBuild, "build|learn")
	c.Flags().BoolVarP(&flagGlobal, "global", "g", false, "install content-producing skills as personal, cross-project content instead of project-local/vendored (I1: --global no longer means \"the normal path\")")
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
		Mode:            flagMode,
		Global:          flagGlobal,
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
	vaultDir, err := raypaths.VaultDir()
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
	return initai.Home{
		ProfilesDir:  profilesDir,
		TemplatesDir: templatesDir,
		VaultDir:     vaultDir,
		ConfigPath:   configPath,
		StatePath:    statePath,
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
}
