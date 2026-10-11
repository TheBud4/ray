package cmd

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/scaffold"
	"github.com/TheBud4/ray/internal/store"
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
