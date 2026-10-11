package cmd

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/catalog"
	"github.com/TheBud4/ray/internal/scaffold"
	"github.com/TheBud4/ray/internal/store"
	"github.com/TheBud4/ray/internal/testenv"
)

const tmplSample = "CLAUDE.md.tmpl"

func embeddedTemplate(t *testing.T, rel string) []byte {
	t.Helper()
	b, err := fs.ReadFile(scaffold.EmbeddedTemplates(), rel)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func seedOverlay(t *testing.T, home, rel, content string) {
	t.Helper()
	p := filepath.Join(home, "templates", filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func seedBaseline(t *testing.T, home, rel string, content []byte) {
	t.Helper()
	st := store.New(filepath.Join(home, "store"))
	if err := st.SetPristine(filepath.Join(home, "templates"), rel, store.HashBytes(content)); err != nil {
		t.Fatal(err)
	}
}

// stateOf lê a coluna de estado da linha de name numa saída do list.
func stateOf(out, name string) string {
	for _, l := range strings.Split(out, "\n") {
		if f := strings.Fields(l); len(f) > 1 && f[0] == name {
			return strings.Join(f[1:], " ")
		}
	}
	return ""
}

func TestTemplateListShowsTheStateOfEachTemplate(t *testing.T) {
	home := dryRunHome(t)
	out, err := execRoot(t, "template", "list")
	if err != nil {
		t.Fatalf("list error = %v", err)
	}
	if !strings.HasPrefix(out, "NAME") || stateOf(out, tmplSample) != "new" {
		t.Errorf("empty overlay: output =\n%s", out)
	}

	seedOverlay(t, home, tmplSample, "mine\n")
	out, _ = execRoot(t, "template", "list")
	if stateOf(out, tmplSample) != "edited (no baseline)" {
		t.Errorf("edited without baseline: output =\n%s", out)
	}
	if !strings.Contains(out, "1 edited · ") {
		t.Errorf("summary line missing:\n%s", out)
	}
}

func TestTemplateShowPrintsOnlyTheContentInitAiWouldUse(t *testing.T) {
	home := dryRunHome(t)
	out, err := execRoot(t, "template", "show", tmplSample)
	if err != nil || out != string(embeddedTemplate(t, tmplSample)) {
		t.Errorf("show (new) = %q, %v; want exactly the embedded template", out, err)
	}
	seedOverlay(t, home, tmplSample, "mine\n")
	if out, _ := execRoot(t, "template", "show", tmplSample); out != "mine\n" {
		t.Errorf("show (edited) = %q, want the overlay", out)
	}
}

func TestTemplatePathPrintsTheDirectoryOrTheFile(t *testing.T) {
	home := dryRunHome(t)
	out, _ := execRoot(t, "template", "path")
	if strings.TrimSpace(out) != filepath.Join(home, "templates") {
		t.Errorf("path = %q", out)
	}
	out, _ = execRoot(t, "template", "path", tmplSample)
	if strings.TrimSpace(out) != filepath.Join(home, "templates", tmplSample) {
		t.Errorf("path name = %q", out)
	}
}

func TestTemplateNameCompletion(t *testing.T) {
	dryRunHome(t)
	out, err := execRoot(t, "__complete", "template", "show", "")
	if err != nil || !strings.Contains(out, tmplSample) {
		t.Errorf("completion = %q, %v", out, err)
	}
}

func fakeEditor(appendText string) func(editor, path string) error {
	return func(editor, path string) error {
		if appendText == "" {
			return nil
		}
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = f.WriteString(appendText)
		return err
	}
}

func templatesFor(t *testing.T) catalog.Templates {
	t.Helper()
	cat, err := templateCatalog(true)
	if err != nil {
		t.Fatal(err)
	}
	return cat
}

func TestTemplateEditMaterializesValidatesAndReports(t *testing.T) {
	home := dryRunHome(t)
	t.Setenv("EDITOR", "fake")
	cat := templatesFor(t)
	var out, errOut bytes.Buffer

	// new → materializa com a linha-base; um editor que não muda nada.
	if err := runTemplateEdit(cat, tmplSample, fakeEditor(""), false, &out, &errOut); err != nil {
		t.Fatalf("edit error = %v", err)
	}
	if !strings.Contains(out.String(), tmplSample+": no changes") {
		t.Errorf("output = %q, want no changes", out.String())
	}
	st := store.New(filepath.Join(home, "store"))
	if h, ok := st.PristineHash(filepath.Join(home, "templates"), tmplSample); !ok || h != store.HashBytes(embeddedTemplate(t, tmplSample)) {
		t.Errorf("materialization did not record the baseline")
	}

	// edição que renderiza → edited.
	out.Reset()
	if err := runTemplateEdit(cat, tmplSample, fakeEditor("\nmine\n"), false, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), tmplSample+": edited") {
		t.Errorf("output = %q, want edited", out.String())
	}

	// edição que quebra: o arquivo fica salvo e o erro explica.
	err := runTemplateEdit(cat, tmplSample, fakeEditor("\n{{ .Oops"), false, &out, &errOut)
	if err == nil || !strings.Contains(err.Error(), "saved, but it does not render") || !strings.Contains(err.Error(), "ray template reset") {
		t.Fatalf("broken edit error = %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(home, "templates", tmplSample)); !strings.Contains(string(b), "{{ .Oops") {
		t.Error("the broken edit was not kept")
	}
}

func TestTemplateEditFailingEditorSkipsValidation(t *testing.T) {
	dryRunHome(t)
	t.Setenv("EDITOR", "fake")
	cat := templatesFor(t)
	boom := func(editor, path string) error { return os.ErrInvalid }
	var out, errOut bytes.Buffer
	err := runTemplateEdit(cat, tmplSample, boom, false, &out, &errOut)
	if err == nil || strings.Contains(err.Error(), "does not render") || out.Len() != 0 {
		t.Errorf("error = %v, out = %q; want the editor's error and no report", err, out.String())
	}
}

func TestTemplateEditDryRunWritesNothing(t *testing.T) {
	home := dryRunHome(t)
	t.Setenv("EDITOR", "fake")
	out, err := execRoot(t, "template", "edit", tmplSample, "--dry-run")
	if err != nil {
		t.Fatalf("edit --dry-run error = %v", err)
	}
	if !strings.Contains(out, "+ materialize ") || !strings.Contains(out, "+ edit ") {
		t.Errorf("output = %q", out)
	}
	if _, err := os.Stat(filepath.Join(home, "templates")); !os.IsNotExist(err) {
		t.Errorf("dry-run created the overlay: %v", err)
	}
}

func TestTemplateEditWithoutEditorFails(t *testing.T) {
	dryRunHome(t)
	t.Setenv("EDITOR", "")
	if _, err := execRoot(t, "template", "edit", tmplSample); err == nil || !strings.Contains(err.Error(), "$EDITOR is not set") {
		t.Errorf("error = %v", err)
	}
}

func TestEditorCommandSplitsAnEditorWithArguments(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the editor")
	}
	bin := t.TempDir()
	script := filepath.Join(bin, "myeditor")
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	got := editorCommand("myeditor -w", "/tmp/f").Args
	if len(got) != 3 || got[0] != "myeditor" || got[1] != "-w" || got[2] != "/tmp/f" {
		t.Errorf("Args = %v, want [myeditor -w /tmp/f]", got)
	}
	got = editorCommand("myeditor", "/tmp/f").Args
	if len(got) != 2 {
		t.Errorf("Args = %v, want the editor alone and the path", got)
	}
}

// Um edit --dry-run não promete o que a execução real recusa.
func TestTemplateEditDryRunRefusesWhatTheRealRunRefuses(t *testing.T) {
	home := dryRunHome(t)
	t.Setenv("EDITOR", "fake")
	elsewhere := filepath.Join(t.TempDir(), "dotfile")
	if err := os.WriteFile(elsewhere, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, "templates"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(home, "templates", tmplSample)); err != nil {
		testenv.SymlinkUnavailable(t, err)
	}
	seedBaseline(t, home, tmplSample, []byte("keep\n")) // igual à linha-base: outdated

	for _, args := range [][]string{{"template", "edit", tmplSample, "--dry-run"}, {"template", "edit", tmplSample}} {
		out, err := execRoot(t, args...)
		if err == nil || !strings.Contains(err.Error(), "symlink") || strings.Contains(out, "+ materialize") {
			t.Errorf("%v: out = %q, error = %v; want a symlink refusal and no plan", args, out, err)
		}
	}
	if b, _ := os.ReadFile(elsewhere); string(b) != "keep\n" {
		t.Errorf("the symlink target was changed: %q", b)
	}
}

func TestTemplateEditBackToTheDefaultReportsCurrent(t *testing.T) {
	home := dryRunHome(t)
	t.Setenv("EDITOR", "fake")
	cat := templatesFor(t)
	fresh := embeddedTemplate(t, tmplSample)
	seedOverlay(t, home, tmplSample, "mine\n")
	seedBaseline(t, home, tmplSample, fresh)
	restore := func(editor, path string) error { return os.WriteFile(path, fresh, 0o644) }
	var out, errOut bytes.Buffer
	if err := runTemplateEdit(cat, tmplSample, restore, false, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), tmplSample+": current (matches the default)") {
		t.Errorf("output = %q", out.String())
	}
}

func TestTemplateEditWithAnEditorThatDoesNotExistFails(t *testing.T) {
	dryRunHome(t)
	t.Setenv("EDITOR", "ray-no-such-editor-xyz")
	if _, err := execRoot(t, "template", "edit", tmplSample); err == nil {
		t.Error("an $EDITOR that cannot be started must fail the command")
	}
}

func TestTemplateEditWithoutEditorFailsEvenUnderDryRun(t *testing.T) {
	dryRunHome(t)
	t.Setenv("EDITOR", "")
	if _, err := execRoot(t, "template", "edit", tmplSample, "--dry-run"); err == nil || !strings.Contains(err.Error(), "$EDITOR is not set") {
		t.Errorf("error = %v", err)
	}
}
