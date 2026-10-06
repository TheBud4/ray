package store

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// seedTree escreve files (path relativo → conteúdo) sob dir e devolve dir.
func seedTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for rel, content := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestPutGetRoundTrip(t *testing.T) {
	root := t.TempDir()
	src := seedTree(t, map[string]string{
		"SKILL.md":           "# hello",
		"references/deep.md": "detail",
	})

	s := New(root)
	hash, err := s.Put("skills:o/r#skill", src)
	if err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if hash == "" {
		t.Fatal("Put() returned empty hash")
	}

	dir, ok := s.Get("skills:o/r#skill")
	if !ok {
		t.Fatal("Get() ok = false, want true after Put")
	}
	got, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "# hello" {
		t.Errorf("SKILL.md = %q, want %q", got, "# hello")
	}
	got, err = os.ReadFile(filepath.Join(dir, "references/deep.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "detail" {
		t.Errorf("references/deep.md = %q, want %q", got, "detail")
	}
}

func TestGetMissingCoordNotOK(t *testing.T) {
	s := New(t.TempDir())
	if _, ok := s.Get("nope"); ok {
		t.Fatal("Get() ok = true for a coord never Put")
	}
}

func TestHasReflectsIndex(t *testing.T) {
	root := t.TempDir()
	src := seedTree(t, map[string]string{"a.md": "x"})
	s := New(root)

	if s.Has("skills:o/r#a") {
		t.Fatal("Has() = true before Put")
	}
	if _, err := s.Put("skills:o/r#a", src); err != nil {
		t.Fatal(err)
	}
	if !s.Has("skills:o/r#a") {
		t.Fatal("Has() = false after Put")
	}
	if s.Has("skills:o/r#other") {
		t.Fatal("Has() = true for an unrelated coord")
	}
}

func TestStableHashSameContentDifferentSource(t *testing.T) {
	root := t.TempDir()
	srcA := seedTree(t, map[string]string{"SKILL.md": "same bytes"})
	srcB := seedTree(t, map[string]string{"SKILL.md": "same bytes"})

	s := New(root)
	hashA, err := s.Put("git:repo@ref#path", srcA)
	if err != nil {
		t.Fatal(err)
	}
	hashB, err := s.Put("skills:o/r#skill", srcB)
	if err != nil {
		t.Fatal(err)
	}
	if hashA != hashB {
		t.Fatalf("hashA = %q, hashB = %q, want equal for identical content", hashA, hashB)
	}

	entries, err := os.ReadDir(filepath.Join(root, "objects"))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("objects/ has %d entries, want 1 (dedup by content)", len(entries))
	}
}

func TestHashDiffersWithContent(t *testing.T) {
	root := t.TempDir()
	srcA := seedTree(t, map[string]string{"SKILL.md": "version A"})
	srcB := seedTree(t, map[string]string{"SKILL.md": "version B"})

	s := New(root)
	hashA, err := s.Put("coord-a", srcA)
	if err != nil {
		t.Fatal(err)
	}
	hashB, err := s.Put("coord-b", srcB)
	if err != nil {
		t.Fatal(err)
	}
	if hashA == hashB {
		t.Fatal("hashA == hashB, want different hashes for different content")
	}
}

func TestPutSameCoordTwiceIsIdempotent(t *testing.T) {
	root := t.TempDir()
	src := seedTree(t, map[string]string{"SKILL.md": "x"})
	s := New(root)

	h1, err := s.Put("skills:o/r#a", src)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := s.Put("skills:o/r#a", src)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatalf("hash changed across repeated Put of same coord/content: %q vs %q", h1, h2)
	}

	// index.yaml must still have exactly one entry for this coord.
	s2 := New(root)
	if !s2.Has("skills:o/r#a") {
		t.Fatal("Has() = false after two Puts of the same coord")
	}
}

func TestHashTreeSingleFile(t *testing.T) {
	dir := seedTree(t, map[string]string{"skill.md": "content"})
	file := filepath.Join(dir, "skill.md")

	h1, err := HashTree(file)
	if err != nil {
		t.Fatal(err)
	}
	h2, err := HashTree(dir)
	if err != nil {
		t.Fatal(err)
	}
	if h1 != h2 {
		t.Fatalf("HashTree(file) = %q, HashTree(parentDir) = %q, want equal for single-file dir", h1, h2)
	}
}

func TestPristineHashRoundTrip(t *testing.T) {
	s := New(t.TempDir())

	if _, ok := s.PristineHash("/proj/a", "git:o/r#c"); ok {
		t.Fatal("PristineHash() ok = true before SetPristine")
	}

	if err := s.SetPristine("/proj/a", "git:o/r#c", "abc123"); err != nil {
		t.Fatal(err)
	}
	got, ok := s.PristineHash("/proj/a", "git:o/r#c")
	if !ok || got != "abc123" {
		t.Fatalf("PristineHash() = (%q, %v), want (%q, true)", got, ok, "abc123")
	}
}

func TestPristineHashIsolatedPerProject(t *testing.T) {
	s := New(t.TempDir())
	if err := s.SetPristine("/proj/a", "coord", "hash-a"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPristine("/proj/b", "coord", "hash-b"); err != nil {
		t.Fatal(err)
	}

	gotA, _ := s.PristineHash("/proj/a", "coord")
	gotB, _ := s.PristineHash("/proj/b", "coord")
	if gotA != "hash-a" || gotB != "hash-b" {
		t.Fatalf("got (%q, %q), want (%q, %q)", gotA, gotB, "hash-a", "hash-b")
	}
}

func TestSetPristineOverwrites(t *testing.T) {
	s := New(t.TempDir())
	if err := s.SetPristine("/proj/a", "coord", "old"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetPristine("/proj/a", "coord", "new"); err != nil {
		t.Fatal(err)
	}
	got, ok := s.PristineHash("/proj/a", "coord")
	if !ok || got != "new" {
		t.Fatalf("PristineHash() = (%q, %v), want (%q, true)", got, ok, "new")
	}
}

// LocalState responde duas perguntas separadas — "a pasta existe?" e "qual o
// hash dela?" — porque o erro do hash não pode servir de teste de existência:
// o ReadFile de um symlink pendente também devolve "no such file".
func TestLocalState(t *testing.T) {
	t.Run("missing path does not exist", func(t *testing.T) {
		hash, exists := LocalState(filepath.Join(t.TempDir(), "nope"))
		if exists || hash != "" {
			t.Errorf("LocalState() = (%q, %v), want (\"\", false)", hash, exists)
		}
	})
	t.Run("readable tree matches HashTree", func(t *testing.T) {
		dir := seedTree(t, map[string]string{"SKILL.md": "# s", "sub/a.md": "a"})
		want, err := HashTree(dir)
		if err != nil {
			t.Fatal(err)
		}
		hash, exists := LocalState(dir)
		if !exists || hash != want {
			t.Errorf("LocalState() = (%q, %v), want (%q, true)", hash, exists, want)
		}
	})
	t.Run("tree with a dangling symlink exists but has no hash", func(t *testing.T) {
		dir := seedTree(t, map[string]string{"SKILL.md": "# s"})
		if err := os.Symlink(filepath.Join(dir, "missing-target"), filepath.Join(dir, "dangling")); err != nil {
			t.Skipf("symlinks unavailable: %v", err)
		}
		hash, exists := LocalState(dir)
		if !exists || hash != "" {
			t.Errorf("LocalState() = (%q, %v), want (\"\", true): unreadable is not the same as absent", hash, exists)
		}
	})
}

// Um pristine.yaml com `null` é um arquivo sem linhas-base, não um motivo de
// panic: a leitura devolve "não sei" e a gravação recomeça do zero.
func TestPristineTreatsANullFileAsEmpty(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "pristine.yaml"), []byte("null\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	st := New(root)

	if _, ok := st.PristineHash("/proj", "s"); ok {
		t.Error("PristineHash() ok = true on a null file, want false")
	}
	if err := st.SetPristine("/proj", "s", "abc"); err != nil {
		t.Fatalf("SetPristine() error = %v", err)
	}
	if got, ok := st.PristineHash("/proj", "s"); !ok || got != "abc" {
		t.Errorf("PristineHash() = (%q, %v), want (abc, true)", got, ok)
	}
}

// Um symlink dentro do componente é copiado como arquivo com o conteúdo do
// alvo (é o que o HashTree também lê, então o hash e a cópia concordam). O
// modo tem de ser o do alvo: o do próprio symlink é sempre 0777, e o projeto
// receberia um arquivo executável por todos sem que ninguém o tenha pedido.
func TestCopyTreeGivesASymlinkTheModeOfItsTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("modos POSIX")
	}
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "real.md"), []byte("conteúdo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("real.md", filepath.Join(src, "link.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	dst := filepath.Join(t.TempDir(), "out")

	if err := CopyTree(src, dst); err != nil {
		t.Fatalf("CopyTree() error = %v", err)
	}

	info, err := os.Lstat(filepath.Join(dst, "link.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode of the copied symlink = %o, want 600 (its target's)", got)
	}
	if got, _ := os.ReadFile(filepath.Join(dst, "link.md")); string(got) != "conteúdo" {
		t.Errorf("content = %q, want the target's", got)
	}
}
