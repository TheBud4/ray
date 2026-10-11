package catalog

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/scaffold"
	"github.com/TheBud4/ray/internal/store"
	"github.com/TheBud4/ray/internal/testenv"
)

const sample = "CLAUDE.md.tmpl" // um template embutido estável nos testes

// fixture monta um overlay vazio em t.TempDir() com a linha-base num Store real,
// do mesmo jeito que o cmd a liga.
type fixture struct {
	t   *testing.T
	dir string
	st  *store.Store
	cat Templates
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "templates")
	st := store.New(filepath.Join(t.TempDir(), "store"))
	f := &fixture{t: t, dir: dir, st: st}
	f.cat = Templates{
		Dir:         dir,
		Pristine:    func(rel string) (string, bool) { return st.PristineHash(dir, rel) },
		SetPristine: func(rel, hash string) error { return st.SetPristine(dir, rel, hash) },
	}
	return f
}

func (f *fixture) embedded(name string) []byte {
	f.t.Helper()
	b, err := scaffold.EmbeddedTemplates().(interface {
		ReadFile(string) ([]byte, error)
	}).ReadFile(name)
	if err != nil {
		f.t.Fatal(err)
	}
	return b
}

func (f *fixture) overlay(name, content string) {
	f.t.Helper()
	p := filepath.Join(f.dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) baseline(name string, content []byte) {
	f.t.Helper()
	if err := f.st.SetPristine(f.dir, name, store.HashBytes(content)); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) state(name string) Entry {
	f.t.Helper()
	e, err := f.cat.Lookup(name)
	if err != nil {
		f.t.Fatal(err)
	}
	return e
}

func (f *fixture) tree() []string {
	f.t.Helper()
	var out []string
	_ = filepath.WalkDir(f.dir, func(p string, d os.DirEntry, err error) error {
		if err == nil {
			rel, _ := filepath.Rel(f.dir, p)
			out = append(out, rel)
		}
		return nil
	})
	return out
}

// A matriz de estados tem de coincidir com o que o EnsureTemplates faz — o list
// não pode discordar do init ai.
func TestListStateMatchesWhatEnsureTemplatesDoes(t *testing.T) {
	type kase struct {
		name       string
		setup      func(f *fixture, fresh []byte)
		want       State
		noBaseline bool
	}
	cases := []kase{
		{"no overlay", func(f *fixture, fresh []byte) {}, New, false},
		{"equal, no baseline", func(f *fixture, fresh []byte) { f.overlay(sample, string(fresh)) }, Current, false},
		{"equal, matching baseline", func(f *fixture, fresh []byte) {
			f.overlay(sample, string(fresh))
			f.baseline(sample, fresh)
		}, Current, false},
		{"equal, stale baseline", func(f *fixture, fresh []byte) {
			f.overlay(sample, string(fresh))
			f.baseline(sample, []byte("older"))
		}, Current, false},
		{"older version, untouched", func(f *fixture, fresh []byte) {
			f.overlay(sample, "older version\n")
			f.baseline(sample, []byte("older version\n"))
		}, Outdated, false},
		{"edited, baseline is the default", func(f *fixture, fresh []byte) {
			f.overlay(sample, "mine\n")
			f.baseline(sample, fresh)
		}, Edited, false},
		{"differs, no baseline", func(f *fixture, fresh []byte) { f.overlay(sample, "mine\n") }, Edited, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			c.setup(f, f.embedded(sample))
			before := f.tree()

			e := f.state(sample)
			if e.State != c.want || e.NoBaseline != c.noBaseline {
				t.Errorf("state = %s (noBaseline %v), want %s (noBaseline %v)", e.State, e.NoBaseline, c.want, c.noBaseline)
			}
			if after := f.tree(); !slices.Equal(before, after) {
				t.Errorf("List changed the overlay: %v -> %v", before, after)
			}
		})
	}
}

func TestListFindsCustomTemplatesAndIgnoresJunk(t *testing.T) {
	f := newFixture(t)
	f.overlay("mine/extra.md.tmpl", "x")
	for _, junk := range []string{".CLAUDE.md.tmpl.swp", "CLAUDE.md.tmpl~", ".DS_Store", "notes.txt", ".hidden/y.tmpl", "mine/.z.md.tmpl"} {
		f.overlay(junk, "junk")
	}
	entries, err := f.cat.List()
	if err != nil {
		t.Fatal(err)
	}
	var customs []string
	for _, e := range entries {
		if e.State == Custom {
			customs = append(customs, e.Name)
		}
	}
	if !slices.Equal(customs, []string{"mine/extra.md.tmpl"}) {
		t.Errorf("custom = %v, want only mine/extra.md.tmpl", customs)
	}
}

// Só vale um nome exatamente igual a um que o List devolve.
func TestLookupRejectsAnythingButAnExactName(t *testing.T) {
	f := newFixture(t)
	for _, bad := range []string{"../x", "/etc/passwd", "claude/../CLAUDE.md.tmpl", sample + "/", "nope.tmpl", ""} {
		if _, err := f.cat.Lookup(bad); !errors.Is(err, ErrUnknown) {
			t.Errorf("Lookup(%q) error = %v, want ErrUnknown", bad, err)
		}
	}
}

// O init ai sincroniza antes de renderizar: o que ele usaria num outdated é o embutido.
func TestReadReturnsWhatTheNextInitAiWouldUse(t *testing.T) {
	f := newFixture(t)
	fresh := f.embedded(sample)

	f.overlay(sample, "older\n")
	f.baseline(sample, []byte("older\n"))
	if got, _ := f.cat.Read(sample); string(got) != string(fresh) {
		t.Errorf("outdated Read = %q, want the embedded template", got)
	}

	f.overlay(sample, "mine\n")
	if got, _ := f.cat.Read(sample); string(got) != "mine\n" {
		t.Errorf("edited Read = %q, want the overlay", got)
	}
}

// Um overlay mantido por um gerenciador de dotfiles pode ser um symlink: o list
// tem de enxergar o que o init ai enxerga.
func TestListFollowsAnOverlayThatIsASymlink(t *testing.T) {
	f := newFixture(t)
	real := filepath.Join(t.TempDir(), "real")
	if err := os.MkdirAll(filepath.Join(real, "mine"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "mine", "extra.md.tmpl"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(f.dir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, f.dir); err != nil {
		testenv.SymlinkUnavailable(t, err)
	}
	e, err := f.cat.Lookup("mine/extra.md.tmpl")
	if err != nil || e.State != Custom {
		t.Errorf("Lookup through a symlinked overlay = %+v, %v; want a custom template", e, err)
	}
}

// Arquivo vazio no overlay é uma edição como outra qualquer.
func TestAnEmptyOverlayFileIsEditedNotMissing(t *testing.T) {
	f := newFixture(t)
	f.overlay(sample, "")
	if e := f.state(sample); e.State != Edited {
		t.Errorf("state of an empty overlay file = %s, want edited", e.State)
	}
	if got, err := f.cat.Read(sample); err != nil || len(got) != 0 {
		t.Errorf("Read = %q, %v; want empty content", got, err)
	}
}

// No Windows o usuário digita o nome com "\"; a comparação é por "/".
func TestLookupAcceptsBackslashesOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("backslash is a path separator only on Windows")
	}
	f := newFixture(t)
	if _, err := f.cat.Lookup(`claude\hooks\guard-add.sh.tmpl`); err != nil {
		t.Errorf("Lookup with backslashes error = %v", err)
	}
}

func TestPrepareMaterializesNewAndOutdatedWithTheBaseline(t *testing.T) {
	for _, name := range []string{"new", "outdated"} {
		t.Run(name, func(t *testing.T) {
			f := newFixture(t)
			fresh := f.embedded(sample)
			if name == "outdated" {
				f.overlay(sample, "older\n")
				f.baseline(sample, []byte("older\n"))
			}
			wrote, bErr, err := f.cat.Prepare(sample)
			if err != nil || bErr != nil || !wrote {
				t.Fatalf("Prepare = %v, %v, %v", wrote, bErr, err)
			}
			if got, _ := os.ReadFile(filepath.Join(f.dir, sample)); string(got) != string(fresh) {
				t.Errorf("overlay not the embedded template")
			}
			if h, ok := f.st.PristineHash(f.dir, sample); !ok || h != store.HashBytes(fresh) {
				t.Errorf("baseline = %q, %v; want the embedded hash", h, ok)
			}
		})
	}
}

func TestPrepareFixesAStaleBaselineOfACurrentTemplateAndLeavesEditedAlone(t *testing.T) {
	f := newFixture(t)
	fresh := f.embedded(sample)
	f.overlay(sample, string(fresh))
	f.baseline(sample, []byte("older"))
	if wrote, _, err := f.cat.Prepare(sample); err != nil || wrote {
		t.Fatalf("Prepare(current) = %v, %v", wrote, err)
	}
	if h, _ := f.st.PristineHash(f.dir, sample); h != store.HashBytes(fresh) {
		t.Errorf("baseline not corrected")
	}

	f.overlay(sample, "mine\n")
	if _, _, err := f.cat.Prepare(sample); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(f.dir, sample)); string(got) != "mine\n" {
		t.Errorf("Prepare touched an edited template")
	}
}

// O segundo conjunto de dados (Stack preenchido) exercita o ramo {{if .Stack}}.
func TestValidateRendersWithAndWithoutStack(t *testing.T) {
	f := newFixture(t)
	f.overlay("a.tmpl", "{{ .ProjectName")
	f.overlay("b.tmpl", "{{if .Stack}}{{.Nope}}{{end}}")
	f.overlay("c.tmpl", "ok {{.ProjectName}}")
	if _, err := f.cat.Validate("a.tmpl"); err == nil {
		t.Error("a syntax error should fail Validate")
	}
	if _, err := f.cat.Validate("b.tmpl"); err == nil {
		t.Error("a bad field in the {{if .Stack}} branch should fail Validate")
	}
	if _, err := f.cat.Validate("c.tmpl"); err != nil {
		t.Errorf("a good template failed: %v", err)
	}
}

func TestValidateWarnsAboutCRLFInAHook(t *testing.T) {
	f := newFixture(t)
	f.overlay("hooks/x.sh.tmpl", "echo hi\r\n")
	w, err := f.cat.Validate("hooks/x.sh.tmpl")
	if err != nil || len(w) != 1 || !strings.Contains(w[0], "CRLF") {
		t.Errorf("Validate = %v, %v; want one CRLF warning", w, err)
	}
}

// Só New e Outdated gravam o arquivo; o resto segue o link e o editor escreve
// através dele. A recusa vale onde o ray escreveria, e o --dry-run a antecipa.
func TestPrepareRefusesASymlinkTargetOnlyWhenItWouldWrite(t *testing.T) {
	f := newFixture(t)
	elsewhere := filepath.Join(t.TempDir(), "dotfile")
	if err := os.WriteFile(elsewhere, []byte("keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(f.dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(elsewhere, filepath.Join(f.dir, sample)); err != nil {
		testenv.SymlinkUnavailable(t, err)
	}

	f.baseline(sample, []byte("older")) // difere de ambos: edited
	if _, err := f.cat.PlanEdit(sample); err != nil {
		t.Errorf("PlanEdit(edited through a symlink) error = %v, want none", err)
	}
	if wrote, _, err := f.cat.Prepare(sample); err != nil || wrote {
		t.Errorf("Prepare(edited through a symlink) = %v, %v; want no write and no error", wrote, err)
	}

	f.baseline(sample, []byte("keep\n")) // igual à linha-base: outdated, o ray gravaria
	if _, err := f.cat.PlanEdit(sample); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Errorf("PlanEdit(outdated through a symlink) error = %v, want a symlink refusal", err)
	}
	if _, _, err := f.cat.Prepare(sample); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Errorf("Prepare(outdated through a symlink) error = %v, want a symlink refusal", err)
	}
	if got, _ := os.ReadFile(elsewhere); string(got) != "keep\n" {
		t.Errorf("the symlink target was changed: %q", got)
	}
}
