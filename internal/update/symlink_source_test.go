package update

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/runner"
)

// plantLeak põe dentro do componente s um link que o torna inseguro de copiar,
// e devolve o conteúdo que NÃO pode aparecer no projeto.
func plantLeak(t *testing.T, home Home, kind string) (secret string) {
	t.Helper()
	comp := filepath.Join(home.ComponentsDir, "s")
	outside := t.TempDir()
	secretFile := filepath.Join(outside, "chave")
	if err := os.WriteFile(secretFile, []byte("SEGREDO"), 0o600); err != nil {
		t.Fatal(err)
	}
	switch kind {
	case "relative":
		rel, err := filepath.Rel(comp, secretFile)
		if err != nil {
			t.Fatal(err)
		}
		symlinkOrSkip(t, rel, filepath.Join(comp, "leak.txt"))
	case "absolute":
		symlinkOrSkip(t, secretFile, filepath.Join(comp, "leak.txt"))
	case "dangling":
		symlinkOrSkip(t, filepath.Join(outside, "nao-existe"), filepath.Join(comp, "leak.txt"))
	case "sibling component":
		sib := filepath.Join(home.ComponentsDir, "outro")
		if err := os.MkdirAll(sib, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sib, "SKILL.md"), []byte("SEGREDO"), 0o644); err != nil {
			t.Fatal(err)
		}
		symlinkOrSkip(t, filepath.Join("..", "outro", "SKILL.md"), filepath.Join(comp, "leak.txt"))
	case "folder":
		symlinkOrSkip(t, outside, filepath.Join(comp, "pasta"))
	default:
		t.Fatalf("unknown leak kind %q", kind)
	}
	return "SEGREDO"
}

// Um link dentro de um componente é seguido na cópia e vira arquivo comum no
// projeto. Link que sai da pasta do componente é recusado antes de qualquer
// efeito — com e sem --force, no real e no dry-run — e o erro nomeia o
// componente.
func TestRunRefusesAComponentSymlinkThatLeavesTheComponent(t *testing.T) {
	for _, kind := range []string{"relative", "absolute", "dangling", "sibling component", "folder"} {
		for _, force := range []bool{false, true} {
			for _, dry := range []bool{false, true} {
				name := kind
				if force {
					name += " +force"
				}
				if dry {
					name += " +dry-run"
				}
				t.Run(name, func(t *testing.T) {
					home := newHome(t)
					seedComponent(t, home, "# s")
					writeProfile(t, home.ProfilesDir, testProfile())
					plantLeak(t, home, kind)
					target := t.TempDir()
					before := treeOf(t, target)

					opts := Options{Profile: "test", Target: target, Force: force, DryRun: dry, NoGlobal: true, Out: &bytes.Buffer{}}
					_, err := Run(&runner.FakeRunner{}, cleanGitCheck(), opts, home)
					if err == nil {
						t.Fatal("Run() = nil error, want a refusal for the component symlink")
					}
					for _, want := range []string{"refusing to copy component", `"s"`, "symlink"} {
						if !strings.Contains(err.Error(), want) {
							t.Errorf("error = %q, want it to contain %q", err, want)
						}
					}
					if got := treeOf(t, target); !slices.Equal(got, before) {
						t.Errorf("project changed despite the refusal:\nbefore %v\nafter  %v", before, got)
					}
				})
			}
		}
	}
}

// A pasta do componente pode ser ela mesma um symlink para outra pasta.
func TestRunCopiesAComponentWhoseFolderIsASymlink(t *testing.T) {
	home := newHome(t)
	writeProfile(t, home.ProfilesDir, testProfile())
	real := t.TempDir()
	if err := os.WriteFile(filepath.Join(real, "SKILL.md"), []byte("# real"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(home.ComponentsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, real, filepath.Join(home.ComponentsDir, "s"))
	target := t.TempDir()

	opts := Options{Profile: "test", Target: target, NoGlobal: true, Out: &bytes.Buffer{}}
	if _, err := Run(&runner.FakeRunner{}, cleanGitCheck(), opts, home); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if b, _ := os.ReadFile(filepath.Join(target, ".claude", "skills", "s", "SKILL.md")); string(b) != "# real" {
		t.Errorf("copied SKILL.md = %q, want %q", b, "# real")
	}
}

// Controle: um link de arquivo que fica dentro do componente continua sendo
// copiado, como arquivo comum com o conteúdo do alvo.
func TestRunStillCopiesAComponentSymlinkThatStaysInside(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "# s")
	writeProfile(t, home.ProfilesDir, testProfile())
	symlinkOrSkip(t, "SKILL.md", filepath.Join(home.ComponentsDir, "s", "alias.md"))
	target := t.TempDir()

	opts := Options{Profile: "test", Target: target, NoGlobal: true, Out: &bytes.Buffer{}}
	if _, err := Run(&runner.FakeRunner{}, cleanGitCheck(), opts, home); err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	alias := filepath.Join(target, ".claude", "skills", "s", "alias.md")
	fi, err := os.Lstat(alias)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Errorf("%s is a symlink, want a regular file with the target's content", alias)
	}
	if b, _ := os.ReadFile(alias); string(b) != "# s" {
		t.Errorf("alias.md = %q, want %q", b, "# s")
	}
}
