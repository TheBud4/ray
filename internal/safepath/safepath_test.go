package safepath

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newRoot devolve um projeto e uma pasta "fora" dele, ambos já resolvidos
// (o t.TempDir pode estar atrás de um symlink, como /tmp no macOS).
func newRoot(t *testing.T) (root, outside string) {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root = filepath.Join(base, "project")
	outside = filepath.Join(base, "outside")
	for _, d := range []string{root, outside} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root, outside
}

func link(t *testing.T, target, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Skipf("symlink not supported here: %v", err)
	}
}

func TestResolveInsideAcceptsPathsThatStayInside(t *testing.T) {
	root, _ := newRoot(t)
	if err := os.MkdirAll(filepath.Join(root, ".claude", "skills"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "real.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	link(t, "real.txt", filepath.Join(root, "rel-link.txt"))             // relativo, interno
	link(t, filepath.Join(root, "real.txt"), filepath.Join(root, "abs")) // absoluto, interno
	link(t, "skills", filepath.Join(root, ".claude", "skills-link"))     // pasta, interna

	for _, p := range []string{
		filepath.Join(root, "nao-existe.json"),                         // destino novo
		filepath.Join(root, "novo", "fundo", "x.json"),                 // pasta nova
		filepath.Join(root, "real.txt"),                                // arquivo comum
		filepath.Join(root, "rel-link.txt"),                            // link relativo interno
		filepath.Join(root, "abs"),                                     // link absoluto interno
		filepath.Join(root, ".claude", "skills-link", "a", "SKILL.md"), // sob pasta-link interna
		root, // a própria raiz
	} {
		if err := ResolveInside(root, p); err != nil {
			t.Errorf("ResolveInside(%s) = %v, want nil", p, err)
		}
	}
}

func TestResolveInsideRefusesSymlinksThatLeaveTheRoot(t *testing.T) {
	root, outside := newRoot(t)
	secret := filepath.Join(outside, "secret")
	if err := os.WriteFile(secret, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link(t, secret, filepath.Join(root, ".claude", ".ray-profile"))                // arquivo, absoluto
	link(t, "../../outside/secret", filepath.Join(root, ".claude", "rel"))         // arquivo, relativo
	link(t, outside, filepath.Join(root, ".claude", "skills"))                     // pasta
	link(t, filepath.Join(outside, "nao-existe"), filepath.Join(root, "dangling")) // pendente
	link(t, "..", filepath.Join(root, "up"))                                       // sobe para o pai

	cases := map[string]string{
		"file symlink, absolute":     filepath.Join(root, ".claude", ".ray-profile"),
		"file symlink, relative":     filepath.Join(root, ".claude", "rel"),
		"directory in the path":      filepath.Join(root, ".claude", "skills", "x", "SKILL.md"),
		"dangling to outside":        filepath.Join(root, "dangling"),
		"link to the parent of root": filepath.Join(root, "up", "outside", "secret"),
	}
	for name, p := range cases {
		err := ResolveInside(root, p)
		if err == nil {
			t.Errorf("%s: ResolveInside(%s) = nil, want error", name, p)
			continue
		}
		if !strings.Contains(err.Error(), p) && !strings.Contains(err.Error(), filepath.Base(p)) {
			t.Errorf("%s: error = %q, want it to name the path", name, err)
		}
	}
}

func TestResolveInsideNamesThePathAndWhereItPoints(t *testing.T) {
	root, outside := newRoot(t)
	link(t, outside, filepath.Join(root, "skills"))

	err := ResolveInside(root, filepath.Join(root, "skills", "a.md"))
	if err == nil {
		t.Fatal("ResolveInside() = nil, want error")
	}
	for _, want := range []string{filepath.Join(root, "skills"), outside} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error = %q, want it to mention %s", err, want)
		}
	}
}

func TestResolveInsideRefusesSymlinkLoops(t *testing.T) {
	root, _ := newRoot(t)
	link(t, "b", filepath.Join(root, "a"))
	link(t, "a", filepath.Join(root, "b"))

	if err := ResolveInside(root, filepath.Join(root, "a")); err == nil {
		t.Fatal("ResolveInside() = nil, want error for a symlink loop")
	}
}

func TestResolveInsideRefusesPathOutsideRootLexically(t *testing.T) {
	root, outside := newRoot(t)
	if err := ResolveInside(root, filepath.Join(outside, "x")); err == nil {
		t.Fatal("ResolveInside() = nil, want error for a path not under root")
	}
}
