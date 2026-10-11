package scaffold

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/TheBud4/ray/internal/profile"
)

func TestEmbeddedTemplatesUsesTheNamesTheOverlayUses(t *testing.T) {
	var names []string
	err := fs.WalkDir(EmbeddedTemplates(), ".", func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			names = append(names, p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(names, "CLAUDE.md.tmpl") || !slices.Contains(names, "claude/hooks/guard-add.sh.tmpl") {
		t.Errorf("names = %v, want paths relative to templates/", names)
	}
}

func TestTemplateForAndTemplateExists(t *testing.T) {
	if name, ok := TemplateFor("CLAUDE.md"); !ok || name != "CLAUDE.md.tmpl" {
		t.Errorf("TemplateFor(CLAUDE.md) = %q, %v", name, ok)
	}
	if _, ok := TemplateFor("nope.md"); ok {
		t.Error("TemplateFor(nope.md) should not resolve")
	}
	dir := t.TempDir()
	if !TemplateExists("CLAUDE.md.tmpl", dir) || TemplateExists("nope.tmpl", dir) {
		t.Error("TemplateExists must see the embedded template and refuse a missing one")
	}
}

func TestDedupFilesLetsTheRecipeWin(t *testing.T) {
	base := []profile.ScaffoldFile{{Path: "a.md", Template: "mine.tmpl"}}
	extra := []profile.ScaffoldFile{{Path: "a.md"}, {Path: "b.md"}}
	got := DedupFiles(base, extra)
	want := []profile.ScaffoldFile{{Path: "a.md", Template: "mine.tmpl"}, {Path: "b.md"}}
	if !slices.Equal(got, want) {
		t.Errorf("DedupFiles = %v, want %v", got, want)
	}
	if len(base) != 1 {
		t.Error("DedupFiles must not modify base")
	}
}
