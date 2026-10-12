package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/catalog"
	"github.com/TheBud4/ray/internal/linediff"
	"github.com/TheBud4/ray/internal/scaffold"
	"github.com/TheBud4/ray/internal/store"
	"github.com/TheBud4/ray/internal/testenv"
	"github.com/TheBud4/ray/internal/ui"
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
	if !strings.Contains(out.String(), "= "+tmplSample+"  no changes") {
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
	if !strings.Contains(out.String(), "~ "+tmplSample+"  edited") {
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
	if !strings.Contains(out.String(), "✓ "+tmplSample+"  current (matches the default)") {
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

// O reset passa pela fiação real (RAY_HOME → store → overlay): a chave da
// linha-base tem de ser a mesma que o init ai lê, senão ele trataria o arquivo
// restaurado como editado.
func TestTemplateResetRecordsTheBaselineInitAiReads(t *testing.T) {
	home := dryRunHome(t)
	fresh := embeddedTemplate(t, tmplSample)
	seedOverlay(t, home, tmplSample, "mine\n")
	// Linha-base velha, diferente do embutido: só um reset que a grave na chave
	// certa deixa o init ai enxergar o arquivo restaurado como intocado.
	seedBaseline(t, home, tmplSample, []byte("an older default"))

	if _, err := execRoot(t, "template", "reset", tmplSample); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("reset of an edited template without --force: error = %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(home, "templates", tmplSample)); string(b) != "mine\n" {
		t.Fatal("a refused reset changed the file")
	}
	if _, err := execRoot(t, "template", "reset", tmplSample, "--force"); err != nil {
		t.Fatalf("reset --force error = %v", err)
	}

	st := store.New(filepath.Join(home, "store"))
	dir := filepath.Join(home, "templates")
	synced, err := scaffold.EnsureTemplates(dir, scaffold.EnsureOptions{
		DryRun:   true,
		Pristine: func(rel string) (string, bool) { return st.PristineHash(dir, rel) },
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range synced {
		if s.Rel == tmplSample && s.Action != scaffold.TemplateCurrent {
			t.Errorf("after reset, init ai would do %q to the template, want current", s.Action)
		}
	}
	if h, ok := st.PristineHash(dir, tmplSample); !ok || h != store.HashBytes(fresh) {
		t.Errorf("baseline = %q, %v; want the embedded hash", h, ok)
	}
}

func TestTemplateResetDryRunPlansWithoutWritingAndKeepsTheRefusals(t *testing.T) {
	home := dryRunHome(t)
	fresh := embeddedTemplate(t, tmplSample)

	out, err := execRoot(t, "template", "reset", tmplSample, "--dry-run")
	if err != nil || !strings.Contains(out, "+ reset "+tmplSample) {
		t.Errorf("dry-run reset of a new template: %q, %v", out, err)
	}
	if _, err := os.Stat(filepath.Join(home, "templates")); !os.IsNotExist(err) {
		t.Errorf("dry-run created the overlay: %v", err)
	}

	seedOverlay(t, home, tmplSample, "mine\n")
	seedBaseline(t, home, tmplSample, fresh)
	if _, err := execRoot(t, "template", "reset", tmplSample, "--dry-run"); err == nil || !strings.Contains(err.Error(), "--force") {
		t.Errorf("the refusal must also hold under --dry-run, got %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(home, "templates", tmplSample)); string(b) != "mine\n" {
		t.Error("dry-run changed the file")
	}
}

// Nome fora do que o list mostra: nenhum comando toca disco.
func TestTemplateUnknownNameIsRefusedByEveryCommandWithoutTouchingDisk(t *testing.T) {
	t.Setenv("EDITOR", "fake")
	for _, name := range []string{"../x", "/etc/passwd", "claude/../CLAUDE.md.tmpl", "nope.tmpl"} {
		for _, sub := range [][]string{{"show"}, {"path"}, {"edit"}, {"diff"}, {"reset"}, {"reset", "--force"}} {
			home := dryRunHome(t)
			args := append([]string{"template", sub[0], name}, sub[1:]...)
			_, err := execRoot(t, args...)
			if err == nil || !strings.Contains(err.Error(), "unknown template") || !strings.Contains(err.Error(), "ray template list") {
				t.Errorf("%v: error = %v", args, err)
			}
			for _, p := range []string{"templates", filepath.Join("store", "pristine.yaml")} {
				if _, serr := os.Stat(filepath.Join(home, p)); !os.IsNotExist(serr) {
					t.Errorf("%v created %s", args, p)
				}
			}
		}
	}
}

func TestTemplateStateCommandsRefuseAnUnreadableBaseline(t *testing.T) {
	home := dryRunHome(t)
	t.Setenv("EDITOR", "fake")
	if err := os.MkdirAll(filepath.Join(home, "store"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "store", "pristine.yaml"), []byte("::: not yaml"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"list"}, {"show", tmplSample}, {"diff", tmplSample}, {"edit", tmplSample}, {"reset", tmplSample}} {
		if _, err := execRoot(t, append([]string{"template"}, args...)...); err == nil || !strings.Contains(err.Error(), "pristine.yaml") {
			t.Errorf("%v: error = %v, want the unreadable-baseline refusal", args, err)
		}
	}
	if _, err := execRoot(t, "template", "path"); err != nil {
		t.Errorf("path should not depend on the baseline: %v", err)
	}
}

func TestTemplateDiffByState(t *testing.T) {
	home := dryRunHome(t)
	fresh := embeddedTemplate(t, tmplSample)

	if out, _ := execRoot(t, "template", "diff", tmplSample); !strings.Contains(out, "not in the overlay yet") {
		t.Errorf("new: %q", out)
	}
	seedOverlay(t, home, tmplSample, string(fresh))
	seedBaseline(t, home, tmplSample, fresh)
	if out, _ := execRoot(t, "template", "diff", tmplSample); !strings.Contains(out, "identical to the embedded template") {
		t.Errorf("current: %q", out)
	}
	seedOverlay(t, home, tmplSample, string(fresh)+"mine\n")
	if out, _ := execRoot(t, "template", "diff", tmplSample); !strings.Contains(out, "--- embedded/"+tmplSample) || !strings.Contains(out, "+mine") {
		t.Errorf("edited: %q", out)
	}
	seedOverlay(t, home, tmplSample, "older\n")
	seedBaseline(t, home, tmplSample, []byte("older\n"))
	var stdout, stderr bytes.Buffer
	if err := runTemplateDiff(templatesFor(t), tmplSample, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stderr.String(), "outdated: the embedded template changed") || strings.Contains(stdout.String(), "outdated:") {
		t.Errorf("outdated: the warning must go to stderr only; stdout = %q, stderr = %q", stdout.String(), stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "--- embedded/") {
		t.Errorf("outdated: stdout must stay a clean diff, got %q", stdout.String())
	}
	seedOverlay(t, home, tmplSample, strings.ReplaceAll(string(fresh), "\n", "\r\n"))
	seedBaseline(t, home, tmplSample, fresh)
	if out, _ := execRoot(t, "template", "diff", tmplSample); !strings.Contains(out, "differs only in line endings (CRLF)") {
		t.Errorf("crlf: %q", out)
	}
	seedOverlay(t, home, "mine/x.md.tmpl", "x")
	if out, _ := execRoot(t, "template", "diff", "mine/x.md.tmpl"); !strings.Contains(out, "custom template") {
		t.Errorf("custom: %q", out)
	}
}

// A tabela do list é a do ui: a coluna NAME alinha pelo maior nome, a última
// célula não é preenchida e, sem cor, não há escape.
func TestTemplateListPlainIsAlignedAndHasNoEscape(t *testing.T) {
	home := dryRunHome(t)
	seedOverlay(t, home, tmplSample, "mine\n")
	cat := templatesFor(t)
	entries, err := cat.List()
	if err != nil {
		t.Fatal(err)
	}
	width := 0
	for _, e := range entries {
		width = max(width, len(e.Name))
	}

	var out bytes.Buffer
	if err := runTemplateList(cat, &out); err != nil {
		t.Fatal(err)
	}
	assertNoEscape(t, out.String())
	lines := strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	if want := fmt.Sprintf("%-*s  STATE", width, "NAME"); lines[0] != want {
		t.Errorf("header = %q, want %q", lines[0], want)
	}
	if want := fmt.Sprintf("%-*s  edited (no baseline)", width, tmplSample); !contains(lines, want) {
		t.Errorf("no line %q in\n%s", want, out.String())
	}
	for _, l := range lines {
		if strings.HasSuffix(l, " ") {
			t.Errorf("line %q has trailing whitespace", l)
		}
	}
	if last := lines[len(lines)-1]; !strings.HasPrefix(last, "1 edited · ") {
		t.Errorf("summary = %q, want it to start with the edited count", last)
	}
}

func contains(lines []string, want string) bool {
	for _, l := range lines {
		if l == want {
			return true
		}
	}
	return false
}

func TestTemplateListColorsTheHeaderTheStateAndTheSummary(t *testing.T) {
	home := dryRunHome(t)
	seedOverlay(t, home, tmplSample, "mine\n")
	var out bytes.Buffer
	if err := runTemplateList(templatesFor(t), ui.New(&out, true)); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	lines := strings.Split(strings.TrimRight(got, "\n"), "\n")
	if !strings.HasPrefix(lines[0], esc+"[90mNAME"+esc+"[0m  ") || !strings.HasSuffix(lines[0], esc+"[90mSTATE"+esc+"[0m") {
		t.Errorf("header = %q, want both cells gray", lines[0])
	}
	if want := esc + "[36medited (no baseline)" + esc + "[0m"; !strings.Contains(got, want) {
		t.Errorf("an edited template must be cyan (%q) in\n%s", want, got)
	}
	if want := esc + "[32mnew" + esc + "[0m"; !strings.Contains(got, want) {
		t.Errorf("a new template must be green (%q) in\n%s", want, got)
	}
	last := lines[len(lines)-1]
	if !strings.HasPrefix(last, esc+"[36m1 edited"+esc+"[0m"+esc+"[90m · "+esc+"[0m"+esc+"[32m") || !strings.HasSuffix(last, " new"+esc+"[0m") {
		t.Errorf("summary = %q, want each count in the color of its state", last)
	}
}

func TestTemplateListColorsOutdatedYellowAndCurrentGray(t *testing.T) {
	home := dryRunHome(t)
	seedOverlay(t, home, tmplSample, "older\n")
	seedBaseline(t, home, tmplSample, []byte("older\n")) // intocado, mas o embutido mudou: outdated
	const same = "SECURITY.md.tmpl"
	seedOverlay(t, home, same, string(embeddedTemplate(t, same)))
	seedBaseline(t, home, same, embeddedTemplate(t, same))
	var out bytes.Buffer
	if err := runTemplateList(templatesFor(t), ui.New(&out, true)); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if want := esc + "[33moutdated" + esc + "[0m"; !strings.Contains(got, want) {
		t.Errorf("outdated must be yellow (%q) in\n%s", want, got)
	}
	if want := esc + "[90mcurrent" + esc + "[0m"; !strings.Contains(got, want) {
		t.Errorf("current must be gray (%q) in\n%s", want, got)
	}
}

func TestTemplateDiffPlainStaysTheUnifiedDiffAndColoredPaintsEachLine(t *testing.T) {
	home := dryRunHome(t)
	fresh := embeddedTemplate(t, tmplSample)
	seedOverlay(t, home, tmplSample, string(fresh)+"mine\n")
	seedBaseline(t, home, tmplSample, fresh)
	want, err := linediff.Unified(string(fresh), string(fresh)+"mine\n", "embedded/"+tmplSample, "overlay/"+tmplSample)
	if err != nil {
		t.Fatal(err)
	}

	var plain bytes.Buffer
	if err := runTemplateDiff(templatesFor(t), tmplSample, &plain, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if plain.String() != want {
		t.Errorf("plain diff =\n%q\nwant exactly\n%q", plain.String(), want)
	}

	var colored bytes.Buffer
	if err := runTemplateDiff(templatesFor(t), tmplSample, ui.New(&colored, true), &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	got := colored.String()
	for _, w := range []string{
		esc + "[90m--- embedded/" + tmplSample + esc + "[0m\n",
		esc + "[90m+++ overlay/" + tmplSample + esc + "[0m\n",
		esc + "[36m@@",
		esc + "[32m+mine" + esc + "[0m\n",
	} {
		if !strings.Contains(got, w) {
			t.Errorf("colored diff lacks %q:\n%q", w, got)
		}
	}
	if stripped := strings.NewReplacer(esc+"[90m", "", esc+"[36m", "", esc+"[32m", "", esc+"[31m", "", esc+"[0m", "").Replace(got); stripped != want {
		t.Errorf("colored diff, once the color is removed, differs from the plain one:\n%q", stripped)
	}
}

func TestTemplateDiffOutdatedNoticeIsAWarningLineOnStderr(t *testing.T) {
	home := dryRunHome(t)
	seedOverlay(t, home, tmplSample, "older\n")
	seedBaseline(t, home, tmplSample, []byte("older\n"))
	var stdout, stderr bytes.Buffer
	if err := runTemplateDiff(templatesFor(t), tmplSample, &stdout, &stderr); err != nil {
		t.Fatal(err)
	}
	if want := "⚠ outdated: the embedded template changed; your file is untouched\n"; stderr.String() != want {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
	assertNoEscape(t, stderr.String())

	stderr.Reset()
	if err := runTemplateDiff(templatesFor(t), tmplSample, &stdout, ui.New(&stderr, true)); err != nil {
		t.Fatal(err)
	}
	if want := esc + "[33m⚠" + esc + "[0m outdated: the embedded template changed; your file is untouched\n"; stderr.String() != want {
		t.Errorf("colored stderr = %q, want %q", stderr.String(), want)
	}
}

func TestTemplateEditResultLinesCarryTheirMarker(t *testing.T) {
	home := dryRunHome(t)
	t.Setenv("EDITOR", "fake")
	cat := templatesFor(t)

	var out, errOut bytes.Buffer
	if err := runTemplateEdit(cat, tmplSample, fakeEditor(""), false, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if want := "= " + tmplSample + "  no changes\n"; out.String() != want {
		t.Errorf("unchanged edit = %q, want %q", out.String(), want)
	}
	assertNoEscape(t, out.String())

	out.Reset()
	if err := runTemplateEdit(cat, tmplSample, fakeEditor("\nmine\n"), false, ui.New(&out, true), &errOut); err != nil {
		t.Fatal(err)
	}
	if want := esc + "[33m~" + esc + "[0m " + tmplSample + "  " + esc + "[90medited" + esc + "[0m\n"; out.String() != want {
		t.Errorf("colored edited = %q, want %q", out.String(), want)
	}

	// Um template custom (sem equivalente embutido) sai com o estado dele.
	seedOverlay(t, home, "mine/x.md.tmpl", "x")
	out.Reset()
	if err := runTemplateEdit(cat, "mine/x.md.tmpl", fakeEditor("y"), false, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if want := "~ mine/x.md.tmpl  custom\n"; out.String() != want {
		t.Errorf("custom edit = %q, want %q", out.String(), want)
	}
}

func TestTemplateEditWarningsAreWarningLines(t *testing.T) {
	dryRunHome(t)
	t.Setenv("EDITOR", "fake")
	const hook = "claude/hooks/guard-add.sh.tmpl"
	crlf := func(editor, path string) error {
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(path, []byte(strings.ReplaceAll(string(b), "\n", "\r\n")), 0o644)
	}
	var out, errOut bytes.Buffer
	if err := runTemplateEdit(templatesFor(t), hook, crlf, false, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if want := "⚠ " + hook + ": CRLF line endings: the hook will fail in bash\n"; errOut.String() != want {
		t.Errorf("stderr = %q, want %q", errOut.String(), want)
	}
	assertNoEscape(t, errOut.String())

	// A linha-base que não pôde ser gravada também é um aviso, não um "warning:".
	cat := templatesFor(t)
	cat.SetPristine = func(rel, hash string) error { return errors.New("disk full") }
	errOut.Reset()
	if err := runTemplateEdit(cat, tmplSample, fakeEditor(""), false, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if got := errOut.String(); !strings.HasPrefix(got, "⚠ "+tmplSample+": baseline not recorded (") || !strings.Contains(got, "disk full") || strings.Contains(got, "warning:") {
		t.Errorf("stderr = %q, want a ⚠ baseline line", got)
	}
}

func TestTemplateResetResultLines(t *testing.T) {
	home := dryRunHome(t)
	cat := templatesFor(t)
	var out, errOut bytes.Buffer

	if err := runTemplateReset(cat, tmplSample, false, false, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if want := "+ " + tmplSample + "  created\n"; out.String() != want {
		t.Errorf("reset of a new template = %q, want %q", out.String(), want)
	}
	out.Reset()
	if err := runTemplateReset(cat, tmplSample, false, false, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if want := "= " + tmplSample + "  already the default\n"; out.String() != want {
		t.Errorf("reset of the default = %q, want %q", out.String(), want)
	}

	seedOverlay(t, home, tmplSample, "mine\n")
	out.Reset()
	if err := runTemplateReset(cat, tmplSample, true, false, ui.New(&out, true), &errOut); err != nil {
		t.Fatal(err)
	}
	if want := esc + "[32m✓" + esc + "[0m " + tmplSample + "  " + esc + "[90mrestored" + esc + "[0m\n"; out.String() != want {
		t.Errorf("colored restored = %q, want %q", out.String(), want)
	}
	assertNoEscape(t, errOut.String())
}

func TestTemplateResetBaselineWarningIsAWarningLine(t *testing.T) {
	dryRunHome(t)
	cat := templatesFor(t)
	cat.SetPristine = func(rel, hash string) error { return errors.New("disk full") }
	var out, errOut bytes.Buffer
	if err := runTemplateReset(cat, tmplSample, false, false, &out, &errOut); err != nil {
		t.Fatal(err)
	}
	if got := errOut.String(); !strings.HasPrefix(got, "⚠ "+tmplSample+": baseline not recorded (") || strings.Contains(got, "warning:") {
		t.Errorf("stderr = %q, want a ⚠ baseline line", got)
	}
}
