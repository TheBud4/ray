package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TheBud4/ray/internal/catalog"
	"github.com/TheBud4/ray/internal/installer"
	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/raypaths"
	"github.com/TheBud4/ray/internal/store"
	"github.com/TheBud4/ray/internal/ui"
)

func newProfileCmd() *cobra.Command {
	c := groupCmd("profile", "Manage recipe profiles")
	c.AddCommand(newProfileListCmd(), newProfileShowCmd(), newProfileAddCmd(),
		newProfileEditCmd(), newProfileRemoveCmd(), newProfilePathCmd())
	return c
}

func newProfileListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Args:  cobra.NoArgs,
		Short: "List available profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := raypaths.ProfilesDir()
			if err != nil {
				return err
			}
			storeDir, err := raypaths.StoreDir()
			if err != nil {
				return err
			}
			return runProfileListWith(dir, storeDir, cmd.OutOrStdout(), !flagDryRun)
		},
	}
}

func runProfileList(dir, storeDir string, out io.Writer) error {
	return runProfileListWith(dir, storeDir, out, true)
}

// runProfileListWith lista as receitas de dir. seed controla se os perfis de
// fábrica ausentes são gravados antes: listar sob --dry-run só lê.
func runProfileListWith(dir, storeDir string, out io.Writer, seed bool) error {
	if seed {
		if err := profile.EnsureDir(dir, store.New(storeDir)); err != nil {
			return err
		}
	}
	entries, err := profile.List(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		switch {
		case e.Unreadable:
			fmt.Fprintf(out, "%s — (unreadable: %s)\n", e.Name, e.Problem)
		case e.Problem != "":
			fmt.Fprintf(out, "%s — %s  (invalid: %s)\n", e.Name, e.Description, e.Problem)
		default:
			fmt.Fprintf(out, "%s — %s\n", e.Name, e.Description)
		}
	}
	return nil
}

func newProfileShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show the resolved plan for a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := raypaths.ProfilesDir()
			if err != nil {
				return err
			}
			cat, err := templateCatalog(true)
			if err != nil {
				return err
			}
			componentsDir, err := raypaths.ComponentsDir()
			if err != nil {
				return err
			}
			p, err := profile.LoadByName(dir, args[0])
			if err != nil {
				return err
			}
			rep, err := cat.Inspect(p, componentsDir)
			if err != nil {
				return err
			}
			return runProfileShowWith(dir, args[0], rep, cmd.OutOrStdout())
		},
	}
}

// runProfileShowWith imprime o plano de um perfil: cada componente e arquivo
// anotado com o que o `init ai` faria, e um veredito no fim.
func runProfileShowWith(dir, name string, rep catalog.Report, out io.Writer) error {
	prof, err := profile.LoadByName(dir, name)
	if err != nil {
		return err
	}
	plan, err := installer.Resolve(prof)
	if err != nil {
		return err
	}

	p := ui.For(out)
	p.Title(prof.Name, prof.Description)

	if len(prof.Components) > 0 {
		items := make([]ui.Item, len(prof.Components))
		for i, c := range prof.Components {
			cr := rep.Components[i]
			if cr.Status == catalog.ComponentOK {
				items[i] = ui.Item{Kind: ui.OK, Name: c.Name, Detail: "-> " + cr.Dest}
			} else {
				items[i] = ui.Item{Kind: ui.Fail, Name: c.Name, Detail: shortHome(cr.Detail)}
			}
		}
		p.Blank()
		p.Section("Components")
		p.Items(items)
	}
	if len(rep.Files) > 0 {
		items := make([]ui.Item, len(rep.Files))
		for i, f := range rep.Files {
			switch {
			case f.Template == "":
				items[i] = ui.Item{Kind: ui.Fail, Name: f.Path, Detail: f.Problem}
			case f.Problem != "":
				items[i] = ui.Item{Kind: ui.Fail, Name: f.Path, Detail: "<- " + f.Template + "  " + f.Problem}
			default:
				items[i] = ui.Item{Kind: ui.OK, Name: f.Path, Detail: "<- " + f.Template + "  (" + string(f.State) + ")"}
			}
		}
		p.Blank()
		p.Section("Files")
		p.Items(items)
	}
	if len(plan.Globals) > 0 {
		p.Blank()
		p.Section("Globals (install-once)")
		for _, g := range plan.Globals {
			for _, c := range g.Commands {
				p.Line("  [" + g.Key + "] " + c.String())
			}
		}
	}
	if len(plan.Servers) > 0 {
		p.Blank()
		p.Section("MCP servers")
		for _, s := range plan.Servers {
			// Command e Args juntos numa lista só: interpolar os args num %s
			// separado pendurava um espaço no fim da linha do servidor que não
			// tem nenhum.
			p.Line("  " + s.Name + ": " + strings.Join(append([]string{s.Command}, s.Args...), " "))
		}
	}

	p.Blank()
	if n := rep.Problems(); n == 0 {
		p.Verdict(ui.OK, "ready", "")
	} else {
		noun := "problems"
		if n == 1 {
			noun = "problem"
		}
		p.Verdict(ui.Fail, fmt.Sprintf("%d %s", n, noun), fmt.Sprintf("`ray init ai --profile %s` would refuse or skip them", name))
	}
	return nil
}

func newProfileAddCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "add <name>",
		Short: "Create a new starter profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := raypaths.ProfilesDir()
			if err != nil {
				return err
			}
			if flagDryRun {
				return planProfileAdd(dir, args[0], cmd.OutOrStdout())
			}
			return runProfileAdd(dir, args[0])
		},
	}
}

func runProfileAdd(dir, name string) error {
	return profile.WriteNew(dir, profile.Starter(name))
}

// planProfileAdd valida o que `profile add` gravaria e só imprime o plano.
func planProfileAdd(dir, name string, out io.Writer) error {
	if err := profile.Starter(name).Validate(); err != nil {
		return err
	}
	path, err := profile.PathFor(dir, name)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "+ add profile %s (%s)\n", name, path)
	return nil
}

func newProfileEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "edit <name>",
		Short: "Open a profile in $EDITOR",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := raypaths.ProfilesDir()
			if err != nil {
				return err
			}
			spawn := spawnEditor
			if flagDryRun {
				// O editor herda o terminal e não passa pelo runner: sob
				// --dry-run o spawn só imprime o que abriria.
				spawn = func(editor, path string) error {
					fmt.Fprintf(cmd.OutOrStdout(), "+ edit %s (with %s)\n", path, editor)
					return nil
				}
			}
			return runProfileEdit(dir, args[0], spawn)
		},
	}
}

// runProfileEdit isola a checagem de $EDITOR do spawn em si, que precisa
// herdar o terminal — não passa pelo runner (ver Fase 9, decisão de design).
func runProfileEdit(dir, name string, spawn func(editor, path string) error) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		return fmt.Errorf("$EDITOR is not set")
	}
	path, err := profile.PathFor(dir, name)
	if err != nil {
		return err
	}
	return spawn(editor, path)
}

// editorCommand monta o comando do editor. Um $EDITOR com argumentos (`code -w`)
// não é um executável: se LookPath não o acha inteiro, é quebrado em palavras,
// sem shell.
func editorCommand(editor, path string) *exec.Cmd {
	if _, err := exec.LookPath(editor); err != nil {
		if words := strings.Fields(editor); len(words) > 1 {
			return exec.Command(words[0], append(words[1:], path)...)
		}
	}
	return exec.Command(editor, path)
}

func spawnEditor(editor, path string) error {
	c := editorCommand(editor, path)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c.Run()
}

func newProfileRemoveCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := raypaths.ProfilesDir()
			if err != nil {
				return err
			}
			if flagDryRun {
				path, err := profile.PathFor(dir, args[0])
				if err != nil {
					return err
				}
				if err := profile.CheckRemovable(args[0]); err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "+ remove profile %s (%s)\n", args[0], path)
				return nil
			}
			return runProfileRemove(dir, args[0])
		},
	}
}

func runProfileRemove(dir, name string) error {
	return profile.Remove(dir, name)
}

func newProfilePathCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "path",
		Args:  cobra.NoArgs,
		Short: "Print the profiles directory",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := raypaths.ProfilesDir()
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), dir)
			return nil
		},
	}
}
