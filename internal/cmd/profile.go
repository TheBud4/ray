package cmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TheBud4/ray/internal/acquire"
	"github.com/TheBud4/ray/internal/installer"
	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/rayconfig"
	"github.com/TheBud4/ray/internal/raypaths"
)

func newProfileCmd() *cobra.Command {
	c := &cobra.Command{Use: "profile", Short: "Manage recipe profiles"}
	c.AddCommand(newProfileListCmd(), newProfileShowCmd(), newProfileAddCmd(),
		newProfileEditCmd(), newProfileRemoveCmd(), newProfilePathCmd())
	return c
}

func newProfileListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List available profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			dir, err := raypaths.ProfilesDir()
			if err != nil {
				return err
			}
			return runProfileList(dir, cmd.OutOrStdout())
		},
	}
}

func runProfileList(dir string, out io.Writer) error {
	if err := profile.EnsureDir(dir); err != nil {
		return err
	}
	entries, err := profile.List(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		fmt.Fprintf(out, "%s — %s\n", e.Name, e.Description)
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
			configPath, err := raypaths.ConfigPath()
			if err != nil {
				return err
			}
			cfg, err := rayconfig.Load(configPath)
			if err != nil {
				return err
			}
			return runProfileShow(dir, args[0], cfg.BrainPath(), cmd.OutOrStdout())
		},
	}
}

func runProfileShow(dir, name, brainPath string, out io.Writer) error {
	p, err := profile.Load(filepath.Join(dir, name+".yaml"))
	if err != nil {
		return err
	}
	plan, err := installer.Resolve(p, installer.Options{BrainPath: brainPath})
	if err != nil {
		return err
	}

	fmt.Fprintf(out, "%s — %s\n", p.Name, p.Description)

	if len(p.Components) > 0 {
		fmt.Fprintln(out, "\nComponents:")
		for _, c := range p.Components {
			cmd, err := acquire.PreviewCommand(c)
			if err != nil {
				return err
			}
			fmt.Fprintf(out, "  %s\n", cmd.String())
		}
	}
	if len(plan.Globals) > 0 {
		fmt.Fprintln(out, "\nGlobals (install-once):")
		for _, g := range plan.Globals {
			for _, c := range g.Commands {
				fmt.Fprintf(out, "  [%s] %s\n", g.Key, c.String())
			}
		}
	}
	if len(plan.Servers) > 0 {
		fmt.Fprintln(out, "\nMCP servers:")
		for _, s := range plan.Servers {
			// Command e Args juntos numa lista só: interpolar os args num %s
			// separado pendurava um espaço no fim da linha do servidor que não
			// tem nenhum.
			fmt.Fprintf(out, "  %s: %s\n", s.Name, strings.Join(append([]string{s.Command}, s.Args...), " "))
		}
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
			return runProfileAdd(dir, args[0])
		},
	}
}

func runProfileAdd(dir, name string) error {
	return profile.WriteNew(dir, profile.Starter(name))
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
			return runProfileEdit(dir, args[0], spawnEditor)
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
	path := filepath.Join(dir, name+".yaml")
	return spawn(editor, path)
}

func spawnEditor(editor, path string) error {
	c := exec.Command(editor, path)
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
