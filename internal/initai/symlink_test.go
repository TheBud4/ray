package initai

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/testenv"
)

// treeOf lista tudo sob root (arquivos, pastas e symlinks, sem seguir link),
// para comparar o antes e o depois de uma execução que deveria recusar.
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

// cleanRun monta o que um `init ai` escreve num projeto vazio e devolve os
// caminhos relativos de todo arquivo e pasta criados. É a fonte da enumeração:
// um destino novo que o init passe a escrever entra aqui sozinho, sem lista
// mantida à mão.
func cleanRun(t *testing.T, home Home) (files, dirs []string) {
	t.Helper()
	target := t.TempDir()
	opts := Options{Profile: "test", Target: target, NoGlobal: true, Out: &bytes.Buffer{}}
	if _, err := Run(&runner.FakeRunner{}, allFound, opts, home); err != nil {
		t.Fatalf("clean Run() error = %v", err)
	}
	for _, rel := range treeOf(t, target) {
		if rel == "." {
			continue
		}
		fi, err := os.Lstat(filepath.Join(target, rel))
		if err != nil {
			t.Fatal(err)
		}
		if fi.IsDir() {
			dirs = append(dirs, rel)
		} else {
			files = append(files, rel)
		}
	}
	if len(files) == 0 || len(dirs) == 0 {
		t.Fatalf("clean run produced files=%v dirs=%v, want both", files, dirs)
	}
	return files, dirs
}

func symlinkOrSkip(t *testing.T, target, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		testenv.SymlinkUnavailable(t, err)
	}
}

// Um clone pode trazer qualquer destino do init como symlink para fora do
// projeto. Para CADA arquivo e pasta que o init escreve: com o symlink no
// lugar, o init (real e dry-run) recusa, o alvo fora fica intacto e nada novo
// aparece no projeto.
func TestRunRefusesEveryDestinationThatIsASymlinkOutsideTheProject(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())
	files, dirs := cleanRun(t, home)

	type kase struct {
		rel   string
		isDir bool
	}
	var cases []kase
	for _, f := range files {
		cases = append(cases, kase{f, false})
	}
	for _, d := range dirs {
		cases = append(cases, kase{d, true})
	}

	for _, c := range cases {
		for _, dry := range []bool{false, true} {
			name := c.rel
			if dry {
				name += " (dry-run)"
			}
			t.Run(name, func(t *testing.T) {
				outside := t.TempDir()
				var linkTo string
				if c.isDir {
					linkTo = outside
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

				opts := Options{Profile: "test", Target: target, NoGlobal: true, DryRun: dry, Out: &bytes.Buffer{}}
				_, err := Run(&runner.FakeRunner{}, allFound, opts, home)
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
				}
			})
		}
	}
}

// Controle: o symlink que fica dentro do projeto (monorepo com .claude
// compartilhado) continua funcionando — a recusa é só para o que sai.
func TestRunFollowsASymlinkThatStaysInsideTheProject(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	shared := filepath.Join(target, "shared")
	if err := os.MkdirAll(shared, 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, "../shared", filepath.Join(target, ".claude", "skills"))

	opts := Options{Profile: "test", Target: target, NoGlobal: true, Out: &bytes.Buffer{}}
	if _, err := Run(&runner.FakeRunner{}, allFound, opts, home); err != nil {
		t.Fatalf("Run() error = %v, want an internal symlink to keep working", err)
	}
	if _, err := os.Stat(filepath.Join(shared, "s", "SKILL.md")); err != nil {
		t.Errorf("component not written through the internal symlink: %v", err)
	}
}

// O probe de gravabilidade escreve no projeto antes da enumeração do teste
// acima, que só vê o que sobra depois de apagado. Um clone que traga o nome do
// probe como symlink para fora não pode fazer o init truncar o alvo nem criar
// arquivo lá.
func TestRunProbeNeverWritesThroughAPlantedSymlink(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())

	for _, dangling := range []bool{false, true} {
		name := "existing target"
		if dangling {
			name = "dangling target"
		}
		t.Run(name, func(t *testing.T) {
			outside := t.TempDir()
			victim := filepath.Join(outside, "victim")
			if !dangling {
				if err := os.WriteFile(victim, []byte("SENTINEL"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			target := t.TempDir()
			symlinkOrSkip(t, victim, filepath.Join(target, ".ray-write-test"))

			opts := Options{Profile: "test", Target: target, NoGlobal: true, Out: &bytes.Buffer{}}
			if _, err := Run(&runner.FakeRunner{}, allFound, opts, home); err != nil {
				t.Fatalf("Run() error = %v", err)
			}

			b, err := os.ReadFile(victim)
			switch {
			case dangling && err == nil:
				t.Errorf("the probe created %s through the dangling symlink", victim)
			case !dangling && string(b) != "SENTINEL":
				t.Errorf("victim = %q, want it untouched (the probe truncated it)", b)
			}
		})
	}
}
