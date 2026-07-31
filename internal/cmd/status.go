package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TheBud4/ray/internal/raypaths"
	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/status"
)

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status [path]",
		Short: "Diagnose the vendored AI environment: drift, forks and a mutilated .gitignore",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "."
			if len(args) == 1 {
				target = args[0]
			}
			return runStatus(target, cmd.OutOrStdout())
		},
	}
}

// runStatus resolve os caminhos de ~/.ray e imprime o diagnóstico. Devolve
// erro só em falha de leitura: problema achado no ambiente é a saída normal
// do comando, não exit ≠ 0.
func runStatus(target string, out io.Writer) error {
	profilesDir, err := raypaths.ProfilesDir()
	if err != nil {
		return err
	}
	storeDir, err := raypaths.StoreDir()
	if err != nil {
		return err
	}
	rep, err := status.Run(runner.ExecRunner{}, status.Options{Target: target},
		status.Home{ProfilesDir: profilesDir, StoreDir: storeDir})
	if err != nil {
		return err
	}
	printStatus(out, rep)
	return nil
}

// printStatus renderiza os três níveis do design: fato sempre, nota quando o
// estado é esperado mas o usuário precisa saber, e ⚠ só quando algo está
// errado agora. Ambiente são imprime duas linhas — é isso que faz o ⚠
// significar alguma coisa quando aparece.
func printStatus(out io.Writer, rep status.Report) {
	printStatusFacts(out, rep)

	if rep.Git == status.GitNeverTracked && len(rep.AddPaths) > 0 {
		fmt.Fprintln(out, "\nenvironment not versioned yet:")
		// Caminhos por nome, nunca `git add -A`/`.`: o guard-add.sh que o
		// ray instala avisa contra add cego, e o status não pode
		// contradizer o hook.
		fmt.Fprintf(out, "  git add %s\n", strings.Join(rep.AddPaths, " "))
	}

	warnings := statusWarnings(rep)
	if len(warnings) == 0 {
		fmt.Fprintln(out, "\nall in order")
		return
	}
	fmt.Fprintln(out)
	for _, w := range warnings {
		fmt.Fprintf(out, "⚠ %s\n", w)
	}
}

func printStatusFacts(out io.Writer, rep status.Report) {
	printFacts(out, rep.Profile, rep.Inventory)
}

// printFacts imprime a linha de fatos do ambiente: perfil e inventário,
// juntados com · e omitindo o que for zero. Compartilhada com a tela de `ray`
// sem subcomando para que as duas nomeiem as mesmas coisas do mesmo jeito.
func printFacts(out io.Writer, profile string, inv status.Inventory) {
	parts := []string{}
	if profile != "" {
		parts = append(parts, "profile: "+profile)
	}
	for _, p := range []struct {
		n     int
		label string
	}{
		{inv.Skills, "skills"},
		{inv.Agents, "agents"},
		{inv.Commands, "commands"},
		{inv.MCPServers, "MCP servers"},
	} {
		if p.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", p.n, p.label))
		}
	}
	if len(parts) > 0 {
		fmt.Fprintln(out, strings.Join(parts, " · "))
	}
}

// statusWarnings junta tudo que merece ⚠. Componente intocado fica de fora:
// não é notícia, e listá-lo enterraria o que é.
func statusWarnings(rep status.Report) []string {
	var w []string
	if rep.Git == status.GitDirty {
		w = append(w, fmt.Sprintf(".claude/: %d changed and not committed", rep.DirtyN))
	}
	w = append(w, rep.Problems...)
	for _, f := range rep.Forks {
		switch f.State {
		case status.ForkEdited:
			w = append(w, fmt.Sprintf("%s: edited locally — `ray update` will preserve it", f.Coord))
		case status.ForkUnknown:
			w = append(w, fmt.Sprintf("%s: unknown provenance (no pristine baseline)", f.Coord))
		}
	}
	return w
}
