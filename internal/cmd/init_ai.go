package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TheBud4/ray/internal/initai"
	"github.com/TheBud4/ray/internal/preflight"
	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/raypaths"
	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/ui"
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
	c.Flags().StringVar(&flagProfile, "profile", "", "recipe to install (default: base — no stack scaffolding)")
	c.Flags().BoolVar(&flagForce, "force", false, "regenerate scaffold files that already exist (never touches .claude/handoff.md)")
	c.Flags().BoolVar(&flagNoGlobal, "no-global", false, "skip all install-once global steps")
	c.Flags().BoolVar(&flagReinstallGlobal, "reinstall-global", false, "ignore state.yaml and reinstall global steps")
	return c
}

// buildInitAIOptions traduz os flags do comando em initai.Options. Sem
// --profile, cai no perfil `base` — init ai não exige mais escolher
// uma stack para provisionar o ambiente de IA.
func buildInitAIOptions(target string, out io.Writer) initai.Options {
	name := flagProfile
	if name == "" {
		name = profile.BaseName
	}
	return initai.Options{
		Profile:         name,
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
	printInitAISummary(out, "init ai", sum)
	if sum.HadFailure {
		return fmt.Errorf("one or more steps failed; see summary above")
	}
	return nil
}

// printInitAISummary renderiza o resumo do init ai (e do new, que herda a
// tela com o próprio título): seções de entradas, avisos, contagem e, se tudo
// correu bem, os próximos passos.
func printInitAISummary(out io.Writer, title string, sum initai.Summary) {
	p := ui.For(out)
	p.Title(title, sum.Profile)
	printEntrySections(p, sum.Entries)
	printWarnings(p, sum.Warnings, sum.Entries)
	printEntryTally(p, sum.Entries, sum.DryRun)
	printNextSteps(p, sum)
}

// printNextSteps fecha o loop do vendoring (I1): o `ray` escreve o ambiente no
// repositório do usuário, e sem esta instrução ninguém o commita — um .claude/
// no disco de uma pessoa só não é ambiente reproduzível.
//
// Não aparece quando algum passo falhou: mandar commitar um ambiente escrito
// pela metade grava o estado quebrado. Nem em --dry-run: nada foi escrito, e
// `git add` de caminhos que não existem contradiz o "would create" do resumo.
func printNextSteps(p *ui.Printer, sum initai.Summary) {
	if sum.HadFailure || sum.DryRun {
		return
	}
	var steps [][2]string
	if dest := cdDestination(sum.Target); dest != "" {
		// Os passos abaixo valem dentro do projeto; quem acabou de rodar
		// `ray new` ainda está na pasta pai.
		steps = append(steps, [2]string{"cd " + shellQuote(dest), ""})
	}
	if sum.InGitRepo && len(sum.VersionedPaths) > 0 {
		// Caminhos por nome, nunca `git add -A`/`.`: o guard-add.sh que este
		// mesmo comando acabou de instalar avisa contra add cego.
		steps = append(steps,
			[2]string{"git add " + strings.Join(sum.VersionedPaths, " "), ""},
			[2]string{`git commit -m "chore: vendor ai environment"`, ""})
	}
	steps = append(steps, [2]string{"claude", ""})
	p.Blank()
	p.Section("Next steps")
	p.Steps(steps)
}

// cdDestination devolve para onde a pessoa precisa dar `cd` para chegar em
// target: vazio se target é desconhecido ou é o diretório atual; o caminho
// relativo quando target está abaixo do diretório atual; o absoluto nos outros
// casos (inclusive quando o diretório atual não pode ser descoberto).
func cdDestination(target string) string {
	if target == "" {
		return ""
	}
	cwd, err := os.Getwd()
	if err != nil {
		return target
	}
	rel, err := filepath.Rel(cwd, target)
	if err != nil {
		return target
	}
	if rel == "." {
		return ""
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return target
	}
	return rel
}

// shellQuote põe s entre aspas duplas quando tem espaço ou metacaractere de
// shell, escapando `\`, `"`, `$` e crase; caminhos simples saem como estão.
func shellQuote(s string) string {
	safe := s != ""
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_-./@%+=:,", r)) {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, `$`, `\$`, "`", "\\`").Replace(s) + `"`
}
