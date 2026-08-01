package cmd

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"text/template"

	"github.com/spf13/cobra"

	"github.com/TheBud4/ray/internal/initai"
	"github.com/TheBud4/ray/internal/preflight"
	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/raypaths"
	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/scaffold"
)

var flagNoGit bool

func newNewCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "new <profile> <name>",
		Short: "Create a new project from a profile and set up its AI environment",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			profilesDir, err := raypaths.ProfilesDir()
			if err != nil {
				return err
			}
			home, err := resolveInitAIHome()
			if err != nil {
				return err
			}

			checkRunner := runner.ExecRunner{}
			looker := preflight.RunnerLooker{Runner: checkRunner}
			execRunner := runner.ExecRunner{DryRun: flagDryRun, Out: cmd.OutOrStdout()}
			initOpts := buildInitAIOptions("", cmd.OutOrStdout())

			sum, err := runNew(execRunner, looker, profilesDir, args[0], args[1], flagNoGit, flagDryRun, initOpts, home)
			if err != nil {
				return err
			}
			printInitAISummary(cmd.OutOrStdout(), sum)
			if sum.HadFailure {
				return fmt.Errorf("one or more steps failed; see summary above")
			}
			return nil
		},
	}
	c.Flags().StringVar(&flagMode, "mode", scaffold.ModeBuild, "build|learn")
	c.Flags().BoolVarP(&flagGlobal, "global", "g", false, "install content-producing skills globally instead of project-local")
	c.Flags().BoolVar(&flagForce, "force", false, "regenerate scaffold files that already exist (never touches .claude/handoff.md)")
	c.Flags().BoolVar(&flagNoGlobal, "no-global", false, "skip all install-once global steps")
	c.Flags().BoolVar(&flagReinstallGlobal, "reinstall-global", false, "ignore state.yaml and reinstall global steps")
	c.Flags().BoolVar(&flagNoGit, "no-git", false, "skip `git init`")
	return c
}

// runNew cria o projeto (create + git init) e então monta a IA nele via
// initai.Run. Falha num passo de create ou no git init aborta antes de
// montar IA sobre um projeto meio-criado (build guide, "Erros, segurança,
// exit codes").
func runNew(r runner.Runner, l preflight.Looker, profilesDir, profileName, projectName string, noGit, dryRun bool, initOpts initai.Options, home initai.Home) (initai.Summary, error) {
	target := projectName

	empty, err := isEmptyOrMissingDir(target)
	if err != nil {
		return initai.Summary{}, err
	}
	if !empty {
		return initai.Summary{}, fmt.Errorf("target %q already exists and is not empty", target)
	}
	// O MkdirAll não passa pelo runner, então o --dry-run não o alcança
	// sozinho: sem esta guarda, simular um `ray new` deixa a pasta para trás.
	// Out normalizado como o initai.Run faz — chamador pode não ter passado um.
	if dryRun {
		out := initOpts.Out
		if out == nil {
			out = io.Discard
		}
		fmt.Fprintf(out, "+ mkdir -p %s\n", target)
	} else if err := os.MkdirAll(target, 0o755); err != nil {
		return initai.Summary{}, err
	}

	if err := profile.EnsureDir(profilesDir); err != nil {
		return initai.Summary{}, err
	}
	prof, err := profile.LoadByName(profilesDir, profileName)
	if err != nil {
		return initai.Summary{}, err
	}

	for _, step := range prof.Create {
		rendered, err := renderCreateStep(step, projectName)
		if err != nil {
			return initai.Summary{}, err
		}
		fields := strings.Fields(rendered)
		if len(fields) == 0 {
			continue
		}
		res, err := r.Run(context.Background(), runner.Command{Name: fields[0], Args: fields[1:], Dir: target})
		if err != nil {
			return initai.Summary{}, fmt.Errorf("create step %q failed: %w", step, err)
		}
		if res.ExitCode != 0 {
			return initai.Summary{}, fmt.Errorf("create step %q exited with code %d", step, res.ExitCode)
		}
	}

	if !noGit {
		res, err := r.Run(context.Background(), runner.Command{Name: "git", Args: []string{"init", "-q"}, Dir: target})
		if err != nil {
			return initai.Summary{}, fmt.Errorf("git init failed: %w", err)
		}
		if res.ExitCode != 0 {
			return initai.Summary{}, fmt.Errorf("git init exited with code %d", res.ExitCode)
		}
	}

	initOpts.Profile = profileName
	initOpts.Target = target
	return initai.Run(r, l, initOpts, home)
}

func renderCreateStep(step, projectName string) (string, error) {
	t, err := template.New("create").Parse(step)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, struct{ Name string }{Name: projectName}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func isEmptyOrMissingDir(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return true, nil
		}
		return false, err
	}
	return len(entries) == 0, nil
}
