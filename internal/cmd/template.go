package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TheBud4/ray/internal/catalog"
	"github.com/TheBud4/ray/internal/raypaths"
	"github.com/TheBud4/ray/internal/store"
)

func newTemplateCmd() *cobra.Command {
	c := groupCmd("template", "Inspect and edit the scaffold templates in the overlay")
	c.AddCommand(newTemplateListCmd(), newTemplateShowCmd(), newTemplatePathCmd(), newTemplateEditCmd())
	return c
}

// templateCatalog liga o catalog ao overlay e à linha-base reais. verify recusa
// uma linha-base ilegível, como o `init ai`: ninguém decide "editado?" sem ela.
func templateCatalog(verify bool) (catalog.Templates, error) {
	dir, err := raypaths.TemplatesDir()
	if err != nil {
		return catalog.Templates{}, err
	}
	storeDir, err := raypaths.StoreDir()
	if err != nil {
		return catalog.Templates{}, err
	}
	st := store.New(storeDir)
	if verify {
		if err := st.Verify(); err != nil {
			return catalog.Templates{}, err
		}
	}
	return catalog.Templates{
		Dir:         dir,
		Pristine:    func(rel string) (string, bool) { return st.PristineHash(dir, rel) },
		SetPristine: func(rel, hash string) error { return st.SetPristine(dir, rel, hash) },
	}, nil
}

// templateNames completa o nome de um template com o que o `list` mostra.
func templateNames(cmd *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	cat, err := templateCatalog(true)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	entries, err := cat.List()
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name
	}
	return names, cobra.ShellCompDirectiveNoFileComp
}

func newTemplateListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the templates and what `ray init ai` will do with each",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := templateCatalog(true)
			if err != nil {
				return err
			}
			return runTemplateList(cat, cmd.OutOrStdout())
		},
	}
}

func runTemplateList(cat catalog.Templates, out io.Writer) error {
	entries, err := cat.List()
	if err != nil {
		return err
	}
	width := len("NAME")
	for _, e := range entries {
		width = max(width, len(e.Name))
	}
	fmt.Fprintf(out, "%-*s  %s\n", width, "NAME", "STATE")
	counts := map[catalog.State]int{}
	for _, e := range entries {
		state := string(e.State)
		if e.NoBaseline {
			state += " (no baseline)"
		}
		fmt.Fprintf(out, "%-*s  %s\n", width, e.Name, state)
		counts[e.State]++
	}
	var parts []string
	for _, s := range []catalog.State{catalog.Edited, catalog.Outdated, catalog.New, catalog.Custom, catalog.Current} {
		if counts[s] > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", counts[s], s))
		}
	}
	fmt.Fprintln(out, strings.Join(parts, " · "))
	return nil
}

func newTemplateShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "show <name>",
		Short:             "Print the template `ray init ai` would use",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: templateNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := templateCatalog(true)
			if err != nil {
				return err
			}
			b, err := cat.Read(args[0])
			if err != nil {
				return err
			}
			_, err = cmd.OutOrStdout().Write(b)
			return err
		},
	}
}

func newTemplatePathCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "path [name]",
		Short:             "Print the overlay directory, or the path of one template",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: templateNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := templateCatalog(false)
			if err != nil {
				return err
			}
			if len(args) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), cat.Dir)
				return nil
			}
			p, err := cat.Path(args[0])
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), p)
			return nil
		},
	}
}

func newTemplateEditCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "edit <name>",
		Short:             "Open a template in $EDITOR and check that it still renders",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: templateNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := templateCatalog(true)
			if err != nil {
				return err
			}
			spawn := spawnEditor
			if flagDryRun {
				spawn = func(editor, path string) error {
					fmt.Fprintf(cmd.OutOrStdout(), "+ edit %s (with %s)\n", path, editor)
					return nil
				}
			}
			return runTemplateEdit(cat, args[0], spawn, flagDryRun, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}

// runTemplateEdit isola a checagem de $EDITOR e o spawn, que herda o terminal e
// não passa pelo runner, como no `profile edit`.
func runTemplateEdit(cat catalog.Templates, name string, spawn func(editor, path string) error, dryRun bool, out, errOut io.Writer) error {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		return fmt.Errorf("$EDITOR is not set")
	}
	entry, err := cat.Lookup(name)
	if err != nil {
		return err
	}
	path, err := cat.Path(entry.Name)
	if err != nil {
		return err
	}
	if dryRun {
		if _, err := cat.PlanEdit(entry.Name); err != nil {
			return err
		}
		if entry.State == catalog.New || entry.State == catalog.Outdated {
			fmt.Fprintf(out, "+ materialize %s\n", path)
		}
		return spawn(editor, path)
	}

	_, baselineErr, err := cat.Prepare(entry.Name)
	if err != nil {
		return err
	}
	if baselineErr != nil {
		fmt.Fprintf(errOut, "warning: %s: baseline not recorded (%v)\n", entry.Name, baselineErr)
	}
	before, err := cat.Overlay(entry.Name)
	if err != nil {
		return err
	}
	if err := spawn(editor, path); err != nil {
		return err
	}
	after, err := cat.Overlay(entry.Name)
	if err != nil {
		return err
	}

	warnings, verr := cat.Validate(entry.Name)
	if verr != nil {
		return fmt.Errorf("%s saved, but it does not render:\n  %v\nfix it with `ray template edit %s`, or restore the default with `ray template reset %s`",
			entry.Name, verr, entry.Name, entry.Name)
	}
	for _, w := range warnings {
		fmt.Fprintf(errOut, "warning: %s: %s\n", entry.Name, w)
	}
	if bytes.Equal(before, after) {
		fmt.Fprintf(out, "%s: no changes\n", entry.Name)
		return nil
	}
	now, err := cat.Lookup(entry.Name)
	if err != nil {
		return err
	}
	if now.State == catalog.Current {
		fmt.Fprintf(out, "%s: current (matches the default)\n", entry.Name)
		return nil
	}
	fmt.Fprintf(out, "%s: %s\n", entry.Name, now.State)
	return nil
}
