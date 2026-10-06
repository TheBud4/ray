package update

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/store"
)

func treeOf(t *testing.T, root string) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		out = append(out, rel)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(out)
	return out
}

func symlinkOrSkip(t *testing.T, target, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlink not supported here: %v", err)
	}
}

// Um clone pode trazer o destino do componente, uma pasta do caminho ou a
// linha-base como symlink para fora do projeto. O update (com e sem --force,
// real e dry-run) recusa antes de copiar ou apagar qualquer coisa: o `--force`
// roda RemoveAll e a cópia grava, e nenhum dos dois pode alcançar fora.
func TestRunRefusesADestinationThatIsASymlinkOutsideTheProject(t *testing.T) {
	cases := []struct {
		name  string
		rel   string // onde o symlink é posto, relativo ao projeto
		isDir bool
	}{
		{"claude dir", ".claude", true},
		{"dest dir", ".claude/skills", true},
		{"component dir", ".claude/skills/s", true},
		{"component file", ".claude/skills/s/SKILL.md", false},
		{"baseline file", ".claude/.ray-pristine.yaml", false},
	}
	for _, c := range cases {
		for _, force := range []bool{false, true} {
			for _, dry := range []bool{false, true} {
				name := c.name
				if force {
					name += " +force"
				}
				if dry {
					name += " +dry-run"
				}
				t.Run(name, func(t *testing.T) {
					home := newHome(t)
					seedComponent(t, home, "# new upstream")
					writeProfile(t, home.ProfilesDir, testProfile())

					outside := t.TempDir()
					var linkTo string
					if c.isDir {
						linkTo = outside
						if err := os.WriteFile(filepath.Join(outside, "keep.txt"), []byte("KEEP"), 0o644); err != nil {
							t.Fatal(err)
						}
					} else {
						linkTo = filepath.Join(outside, "sentinel")
						if err := os.WriteFile(linkTo, []byte("SENTINEL"), 0o644); err != nil {
							t.Fatal(err)
						}
					}
					target := t.TempDir()
					symlinkOrSkip(t, linkTo, filepath.Join(target, c.rel))
					before := treeOf(t, target)
					outsideBefore := treeOf(t, outside)

					opts := Options{Profile: "test", Target: target, Force: force, DryRun: dry, NoGlobal: true, Out: &bytes.Buffer{}}
					_, err := Run(&runner.FakeRunner{}, cleanGitCheck(), opts, home)
					if err == nil {
						t.Fatalf("Run() = nil error, want a refusal for symlink %s", c.rel)
					}
					if !strings.Contains(err.Error(), "symlink") {
						t.Errorf("error = %q, want it to say symlink", err)
					}
					if got := treeOf(t, target); !slices.Equal(got, before) {
						t.Errorf("project changed despite the refusal:\nbefore %v\nafter  %v", before, got)
					}
					if got := treeOf(t, outside); !slices.Equal(got, outsideBefore) {
						t.Errorf("outside changed: before %v after %v", outsideBefore, got)
					}
					if !c.isDir {
						if b, _ := os.ReadFile(linkTo); string(b) != "SENTINEL" {
							t.Errorf("sentinel = %q, want it untouched", b)
						}
					} else if b, _ := os.ReadFile(filepath.Join(outside, "keep.txt")); string(b) != "KEEP" {
						t.Errorf("keep.txt = %q, want it untouched", b)
					}
				})
			}
		}
	}
}

// Controle: o symlink que fica dentro do projeto continua funcionando.
func TestRunFollowsASymlinkThatStaysInsideTheProject(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "# new upstream")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	shared := filepath.Join(target, "shared")
	if err := os.MkdirAll(filepath.Join(shared, "s"), 0o755); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(shared, "s", "SKILL.md")
	if err := os.WriteFile(old, []byte("# old"), 0o644); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, "../shared", filepath.Join(target, ".claude", "skills"))
	h, err := store.HashTree(filepath.Join(shared, "s"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ProjectBaseline(target).SetPristine("s", h); err != nil {
		t.Fatal(err)
	}

	opts := Options{Profile: "test", Target: target, NoGlobal: true, Out: &bytes.Buffer{}}
	if _, err := Run(&runner.FakeRunner{}, cleanGitCheck(), opts, home); err != nil {
		t.Fatalf("Run() error = %v, want an internal symlink to keep working", err)
	}
	if b, _ := os.ReadFile(old); string(b) != "# new upstream" {
		t.Errorf("SKILL.md = %q, want the fresh upstream content written through the internal symlink", b)
	}
}
