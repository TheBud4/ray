package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TheBud4/ray/internal/raypaths"
	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/status"
	"github.com/TheBud4/ray/internal/ui"
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
	configPath, err := raypaths.ConfigPath()
	if err != nil {
		return err
	}
	rep, err := status.Run(runner.ExecRunner{}, status.Options{Target: target},
		status.Home{ProfilesDir: profilesDir, StoreDir: storeDir, ConfigPath: configPath})
	if err != nil {
		return err
	}
	printStatus(out, rep)
	return nil
}

// statusWarning é um aviso do diagnóstico: a manchete e, se houver, a dica do
// que fazer. Os dois vêm separados da origem — o cmd nunca parte uma string,
// porque um Problem pode trazer " — " no próprio texto.
type statusWarning struct {
	Headline, Hint string
}

// printStatus renderiza os dois níveis do design: fato e ⚠. O ambiente ainda não
// versionado é ⚠ de propósito: é o estado em que um clone novo não o reproduz, e
// o aviso traz o `git add` pronto. Ambiente versionado e são imprime só o
// título e o veredito — é isso que faz o ⚠ significar alguma coisa quando
// aparece.
func printStatus(out io.Writer, rep status.Report) {
	p := ui.For(out)
	p.Title("status", strings.Join(nonEmpty(rep.Profile, inventoryText(rep.Inventory)), " · "))
	p.Blank()

	warnings := statusWarnings(rep)
	if len(warnings) == 0 {
		p.Verdict(ui.OK, "all in order", "")
		return
	}
	for _, w := range warnings {
		p.Warning(w.Headline, w.Hint)
	}
	p.Blank()
	label := "warnings"
	if len(warnings) == 1 {
		label = "warning"
	}
	p.Summary(ui.Count{N: len(warnings), Label: label, Color: ui.Yellow})
}

// nonEmpty devolve só as partes não vazias, na ordem dada.
func nonEmpty(parts ...string) []string {
	var kept []string
	for _, s := range parts {
		if s != "" {
			kept = append(kept, s)
		}
	}
	return kept
}

// printFacts imprime a linha de fatos do ambiente: o perfil como título e o
// inventário como detalhe; sem perfil, só o inventário. Compartilhada com a
// tela de `ray` sem subcomando para que as duas nomeiem as mesmas coisas do
// mesmo jeito. Devolve se imprimiu alguma linha.
func printFacts(out io.Writer, profile string, inv status.Inventory) bool {
	p := ui.For(out)
	inventory := inventoryText(inv)
	switch {
	case profile != "":
		p.Title(profile, inventory)
	case inventory != "":
		p.Line(inventory)
	default:
		return false
	}
	return true
}

// inventoryText monta o inventário: contagens juntadas com · e sem o que for
// zero.
func inventoryText(inv status.Inventory) string {
	var parts []string
	// Os dois rótulos vêm escritos, não derivados de uma regra sobre o "s"
	// final: um ambiente com uma skill só é comum, e "1 skills" na primeira
	// linha do primeiro comando que a pessoa roda é o tipo de descuido que
	// ela leva para a opinião sobre o resto.
	for _, c := range []struct {
		n         int
		one, many string
	}{
		{inv.Skills, "skill", "skills"},
		{inv.Agents, "agent", "agents"},
		{inv.Commands, "command", "commands"},
		{inv.MCPServers, "MCP server", "MCP servers"},
	} {
		if c.n > 0 {
			label := c.many
			if c.n == 1 {
				label = c.one
			}
			parts = append(parts, fmt.Sprintf("%d %s", c.n, label))
		}
	}
	return strings.Join(parts, " · ")
}

// statusWarnings junta tudo que merece ⚠. Componente intocado fica de fora:
// não é notícia, e listá-lo enterraria o que é.
func statusWarnings(rep status.Report) []statusWarning {
	var w []statusWarning
	if rep.Git == status.GitNeverTracked && len(rep.AddPaths) > 0 {
		// Caminhos por nome, nunca `git add -A`/`.`: o guard-add.sh que o ray
		// instala avisa contra add cego, e o status não pode contradizer o
		// hook.
		w = append(w, statusWarning{
			Headline: "environment not versioned yet",
			Hint:     "git add " + strings.Join(rep.AddPaths, " "),
		})
	}
	if rep.Git == status.GitDirty {
		w = append(w, statusWarning{Headline: fmt.Sprintf(".claude/: %d changed and not committed", rep.DirtyN)})
	}
	for _, problem := range rep.Problems {
		w = append(w, statusWarning{Headline: problem})
	}
	for _, f := range rep.Forks {
		switch f.State {
		case status.ForkEdited:
			w = append(w, statusWarning{Headline: f.Coord + ": edited locally", Hint: "`ray update` will preserve it"})
		case status.ForkUnknown:
			w = append(w, statusWarning{Headline: f.Coord + ": unknown provenance (no pristine baseline)"})
		}
	}
	return w
}
