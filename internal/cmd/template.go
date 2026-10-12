package cmd

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/TheBud4/ray/internal/catalog"
	"github.com/TheBud4/ray/internal/linediff"
	"github.com/TheBud4/ray/internal/raypaths"
	"github.com/TheBud4/ray/internal/store"
	"github.com/TheBud4/ray/internal/ui"
)

func newTemplateCmd() *cobra.Command {
	c := groupCmd("template", "Inspect and edit the scaffold templates in the overlay")
	c.AddCommand(newTemplateListCmd(), newTemplateShowCmd(), newTemplatePathCmd(),
		newTemplateEditCmd(), newTemplateDiffCmd(), newTemplateResetCmd())
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
	p := ui.For(out)
	rows := make([][]ui.Cell, len(entries))
	counts := map[catalog.State]int{}
	for i, e := range entries {
		state := string(e.State)
		if e.NoBaseline {
			state += " (no baseline)"
		}
		rows[i] = []ui.Cell{{Text: e.Name}, {Text: state, Color: stateColor(e.State)}}
		counts[e.State]++
	}
	p.Table([]string{"NAME", "STATE"}, rows)
	var summary []ui.Count
	for _, s := range []catalog.State{catalog.Edited, catalog.Outdated, catalog.New, catalog.Custom, catalog.Current} {
		summary = append(summary, ui.Count{N: counts[s], Label: string(s), Color: stateColor(s)})
	}
	p.Summary(summary...)
	return nil
}

// stateColor é a cor com que o estado de um template aparece na listagem.
func stateColor(s catalog.State) ui.Color {
	switch s {
	case catalog.Outdated:
		return ui.Yellow
	case catalog.New:
		return ui.Green
	case catalog.Current:
		return ui.Gray
	case catalog.Edited:
		return ui.Cyan
	default:
		return ui.Plain
	}
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
		ui.For(errOut).Warning(fmt.Sprintf("%s: baseline not recorded (%v)", entry.Name, baselineErr), "")
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
		ui.For(errOut).Warning(entry.Name+": "+w, "")
	}
	p := ui.For(out)
	if bytes.Equal(before, after) {
		p.Result(ui.Kept, entry.Name, "no changes")
		return nil
	}
	now, err := cat.Lookup(entry.Name)
	if err != nil {
		return err
	}
	if now.State == catalog.Current {
		p.Result(ui.OK, entry.Name, "current (matches the default)")
		return nil
	}
	p.Result(ui.Updated, entry.Name, string(now.State))
	return nil
}

func newTemplateDiffCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "diff <name>",
		Short:             "Show how the template differs from the one embedded in ray",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: templateNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := templateCatalog(true)
			if err != nil {
				return err
			}
			return runTemplateDiff(cat, args[0], cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}

// runTemplateDiff compara o template do overlay com o embutido. O diff sai limpo
// em out; o aviso de "outdated" vai para errOut para não contaminar um pipe.
func runTemplateDiff(cat catalog.Templates, name string, out, errOut io.Writer) error {
	e, err := cat.Lookup(name)
	if err != nil {
		return err
	}
	p := ui.For(out)
	switch e.State {
	case catalog.Current:
		p.Line("identical to the embedded template")
		return nil
	case catalog.New:
		p.Line("not in the overlay yet; nothing to compare")
		return nil
	case catalog.Custom:
		p.Line("custom template: there is no embedded one to compare to")
		return nil
	}
	embedded, err := cat.Embedded(e.Name)
	if err != nil {
		return err
	}
	overlay, err := cat.Overlay(e.Name)
	if err != nil {
		return err
	}
	if lf := func(b []byte) string { return strings.ReplaceAll(string(b), "\r\n", "\n") }; lf(embedded) == lf(overlay) {
		p.Line("differs only in line endings (CRLF)")
		return nil
	}
	d, err := linediff.Unified(string(embedded), string(overlay), "embedded/"+e.Name, "overlay/"+e.Name)
	if err != nil {
		return err
	}
	if e.State == catalog.Outdated {
		ui.For(errOut).Warning("outdated: the embedded template changed; your file is untouched", "")
	}
	if d != "" {
		for line := range strings.SplitSeq(strings.TrimSuffix(d, "\n"), "\n") {
			p.DiffLine(line)
		}
	}
	return nil
}

func newTemplateResetCmd() *cobra.Command {
	var force bool
	c := &cobra.Command{
		Use:               "reset <name>",
		Short:             "Restore a template to the default embedded in ray",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: templateNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			cat, err := templateCatalog(true)
			if err != nil {
				return err
			}
			return runTemplateReset(cat, args[0], force, flagDryRun, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	c.Flags().BoolVar(&force, "force", false, "discard a local edit")
	return c
}

// changedLines conta as linhas que diferem entre dois conteúdos.
func changedLines(overlay, embedded []byte) int {
	d, err := linediff.Unified(string(embedded), string(overlay), "a", "b")
	if err != nil {
		return -1
	}
	n := 0
	for l := range strings.SplitSeq(d, "\n") {
		if (strings.HasPrefix(l, "+") && !strings.HasPrefix(l, "+++ ")) || (strings.HasPrefix(l, "-") && !strings.HasPrefix(l, "--- ")) {
			n++
		}
	}
	return n
}

func runTemplateReset(cat catalog.Templates, name string, force, dryRun bool, out, errOut io.Writer) error {
	if dryRun {
		e, err := cat.PlanReset(name, force, changedLines)
		if err != nil {
			return err
		}
		p, err := cat.Path(e.Name)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "+ reset %s (%s)\n", e.Name, p)
		return nil
	}
	action, baselineErr, err := cat.Reset(name, force, changedLines)
	if err != nil {
		return err
	}
	if baselineErr != nil {
		ui.For(errOut).Warning(fmt.Sprintf("%s: baseline not recorded (%v)", name, baselineErr), "")
	}
	p := ui.For(out)
	switch action {
	case "created":
		p.Result(ui.Added, name, action)
	case "already the default":
		p.Result(ui.Kept, name, action)
	default:
		p.Result(ui.OK, name, action)
	}
	return nil
}
