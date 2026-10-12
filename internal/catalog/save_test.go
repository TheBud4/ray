package catalog

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/testenv"
)

// read devolve o conteúdo de name no overlay.
func (f *fixture) read(name string) string {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(f.dir, filepath.FromSlash(name)))
	if err != nil {
		f.t.Fatal(err)
	}
	return string(b)
}

func TestSaveWritesAndClassifies(t *testing.T) {
	f := newFixture(t)
	info, _ := f.cat.Info(sample)
	res, err := f.cat.Save(sample, []byte("# mine\n"), info.Hash)
	if err != nil || res.Outcome != Outcome(Edited) || res.State != Edited || res.Render != nil {
		t.Fatalf("Save = %+v, %v; want edited, no render error", res, err)
	}
	if got := f.read(sample); got != "# mine\n" {
		t.Errorf("file = %q, want the saved content", got)
	}
	if st := f.state(sample); st.State != Edited || st.NoBaseline {
		t.Errorf("state after Save = %+v, want edited with a recorded baseline", st)
	}
	if again, _ := f.cat.Info(sample); res.Hash == "" || res.Hash == info.Hash || res.Hash != again.Hash {
		t.Errorf("SaveResult.Hash = %q, want the hash Info reports for the saved file (old %q, Info %q)", res.Hash, info.Hash, again.Hash)
	}
}

func TestSaveKeepsABrokenTemplateAndReportsTheLine(t *testing.T) {
	f := newFixture(t)
	info, _ := f.cat.Info(sample)
	res, err := f.cat.Save(sample, []byte("ok\n{{.Stak}}\n"), info.Hash)
	if err != nil || res.Render == nil || res.Render.Line != 2 {
		t.Fatalf("Save = %+v, %v; want the content kept and a RenderError on line 2", res, err)
	}
	if got := f.read(sample); got != "ok\n{{.Stak}}\n" {
		t.Errorf("a broken template must stay saved, got %q", got)
	}
}

func TestSaveRefusesAStaleBase(t *testing.T) {
	f := newFixture(t)
	info, _ := f.cat.Info(sample)
	f.overlay(sample, "# changed in another tab\n")
	if _, err := f.cat.Save(sample, []byte("# mine\n"), info.Hash); !errors.Is(err, ErrStale) {
		t.Errorf("error = %v, want ErrStale", err)
	}
	if got := f.read(sample); got != "# changed in another tab\n" {
		t.Errorf("a stale save must not write, file = %q", got)
	}
}

func TestSaveRefusesHooksNamesOutsideTheListAndBadUTF8(t *testing.T) {
	f := newFixture(t)
	if _, err := f.cat.Save(hook, []byte("x"), ""); !errors.Is(err, ErrReadOnly) {
		t.Errorf("hook error = %v, want ErrReadOnly", err)
	}
	for _, n := range []string{"../x.tmpl", "/etc/x", "nope.tmpl"} {
		if _, err := f.cat.Save(n, []byte("x"), ""); !errors.Is(err, ErrUnknown) {
			t.Errorf("Save(%q) error = %v, want ErrUnknown", n, err)
		}
	}
	info, _ := f.cat.Info(sample)
	if _, err := f.cat.Save(sample, []byte{0xff, 0xfe}, info.Hash); !errors.Is(err, ErrNotUTF8) {
		t.Errorf("bad UTF-8 error = %v, want ErrNotUTF8", err)
	}
	if entries, _ := os.ReadDir(f.dir); len(entries) != 0 {
		t.Errorf("refused saves created files: %v", entries)
	}
}

func TestSaveRefusesASymlinkAtTheTarget(t *testing.T) {
	f := newFixture(t)
	outside := filepath.Join(t.TempDir(), "outside.tmpl")
	if err := os.WriteFile(outside, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(f.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(f.dir, sample)); err != nil {
		testenv.SymlinkUnavailable(t, err)
		t.Fatal(err)
	}
	info, _ := f.cat.Info(sample)
	if _, err := f.cat.Save(sample, []byte("# mine\n"), info.Hash); err == nil {
		t.Fatal("Save through a symlink succeeded; want it refused")
	}
	if got, _ := os.ReadFile(outside); string(got) != "x" {
		t.Errorf("the symlink target was written: %q", got)
	}
}

func TestSaveRefusesASymlinkedParentDirectory(t *testing.T) {
	f := newFixture(t)
	const nested = "docs/conventions.md.tmpl"
	outside := t.TempDir()
	if err := os.MkdirAll(f.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(f.dir, "docs")); err != nil {
		testenv.SymlinkUnavailable(t, err)
		t.Fatal(err)
	}
	info, _ := f.cat.Info(nested)
	if _, err := f.cat.Save(nested, []byte("# mine\n"), info.Hash); err == nil {
		t.Fatal("Save through a symlinked directory succeeded; want it refused")
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Errorf("the symlinked directory was written to: %v", entries)
	}
}

func TestSaveAnEmptyTemplateIsEdited(t *testing.T) {
	f := newFixture(t)
	info, _ := f.cat.Info(sample)
	res, err := f.cat.Save(sample, nil, info.Hash)
	if err != nil || res.State != Edited {
		t.Errorf("Save(empty) = %+v, %v; want state edited", res, err)
	}
}

func TestSaveRoundTripOfACRLFTemplateIsNoChanges(t *testing.T) {
	f := newFixture(t)
	f.overlay(sample, "a\r\nb\r\n")
	info, _ := f.cat.Info(sample)
	res, err := f.cat.Save(sample, []byte("a\r\nb\r\n"), info.Hash)
	if err != nil || res.Outcome != OutcomeNoChanges {
		t.Errorf("Save = %+v, %v; want Outcome no changes", res, err)
	}
	if got := f.read(sample); got != "a\r\nb\r\n" {
		t.Errorf("file = %q, the CRLF must survive", got)
	}
}

func TestSaveRefusesWhenTheBaselineIsUnreadable(t *testing.T) {
	f := newFixture(t)
	info, _ := f.cat.Info(sample)
	errUnreadable := errors.New("pristine.yaml: unreadable")
	f.cat.Verify = func() error { return errUnreadable }
	// A base é a certa de propósito: sem o Verify, nada mais recusaria.
	if _, err := f.cat.Save(sample, []byte("x"), info.Hash); !errors.Is(err, errUnreadable) {
		t.Errorf("Save error = %v, want the Verify error", err)
	}
	if entries, _ := os.ReadDir(f.dir); len(entries) != 0 {
		t.Errorf("a refused save created files: %v", entries)
	}
}

func TestSaveGivesNoHookWarningForATextTemplateWithCRLF(t *testing.T) {
	// Hooks são somente leitura na web, então o aviso de CRLF em hook nunca sai
	// do Save; um template de texto com CRLF não gera aviso nenhum.
	f := newFixture(t)
	info, _ := f.cat.Info(sample)
	res, err := f.cat.Save(sample, []byte("a\r\nb\r\n"), info.Hash)
	if err != nil || len(res.Warnings) != 0 {
		t.Errorf("Save = %+v, %v; want no warnings for a text template", res, err)
	}
}

func TestSaveBoundsTheRenderOfARecursiveTemplate(t *testing.T) {
	f := newFixture(t)
	info, _ := f.cat.Info(sample)
	// Cada nível chama o anterior duas vezes: sem o teto, a gravação esgotaria a memória.
	var b strings.Builder
	b.WriteString(`{{define "l0"}}xxxxxxxxxx{{end}}`)
	for i := 1; i <= 24; i++ {
		fmt.Fprintf(&b, `{{define "l%d"}}{{template "l%d"}}{{template "l%d"}}{{end}}`, i, i-1, i-1)
	}
	b.WriteString(`{{template "l24"}}`)
	res, err := f.cat.Save(sample, []byte(b.String()), info.Hash)
	if err != nil || res.Render == nil || !strings.Contains(res.Render.Message, "ceiling") {
		t.Fatalf("Save = %+v, %v; want the content kept and a ceiling RenderError", res, err)
	}
}

// Um arquivo que não é UTF-8 não faz o ida-e-volta pelo editor sem perder bytes:
// a gravação por cima dele é recusada, e os bytes ficam como estão.
func TestSaveRefusesToOverwriteAFileThatIsNotUTF8(t *testing.T) {
	f := newFixture(t)
	f.overlay(sample, "ok \xff\xfe\n")
	info, _ := f.cat.Info(sample)
	if info.UTF8 {
		t.Fatal("Info.UTF8 = true for a file with invalid bytes")
	}
	if _, err := f.cat.Save(sample, []byte("ok\n"), info.Hash); !errors.Is(err, ErrNotUTF8) {
		t.Errorf("error = %v, want ErrNotUTF8", err)
	}
	if got := f.read(sample); got != "ok \xff\xfe\n" {
		t.Errorf("the original bytes must survive a refused save, file = %q", got)
	}
}
