package store

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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

// O hash é do conteúdo (caminho relativo + bytes), não de onde a árvore mora.
func TestHashTreeSameContentDifferentSourceSameHash(t *testing.T) {
	srcA := seedTree(t, map[string]string{"SKILL.md": "same bytes"})
	srcB := seedTree(t, map[string]string{"SKILL.md": "same bytes"})

	hashA, err := HashTree(srcA)
	if err != nil {
		t.Fatal(err)
	}
	hashB, err := HashTree(srcB)
	if err != nil {
		t.Fatal(err)
	}
	if hashA != hashB {
		t.Fatalf("hashA = %q, hashB = %q, want equal for identical content", hashA, hashB)
	}
}

func TestHashTreeDiffersWithContent(t *testing.T) {
	srcA := seedTree(t, map[string]string{"SKILL.md": "version A"})
	srcB := seedTree(t, map[string]string{"SKILL.md": "version B"})

	hashA, err := HashTree(srcA)
	if err != nil {
		t.Fatal(err)
	}
	hashB, err := HashTree(srcB)
	if err != nil {
		t.Fatal(err)
	}
	if hashA == hashB {
		t.Fatal("hashA == hashB, want different hashes for different content")
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

// O pristine.yaml é um arquivo só para todos os projetos: perdê-lo, ou ler um
// pedaço dele, faz toda linha-base virar "procedência desconhecida". O
// os.WriteFile trunca antes de escrever, e um leitor que chega nesse intervalo
// vê o arquivo vazio. A gravação tem de ser atômica: ou o conteúdo antigo, ou o
// novo, nunca o meio.
func TestSetPristineIsNeverObservedHalfWritten(t *testing.T) {
	root := t.TempDir()
	s := New(root)
	if err := s.SetPristine("/proj", "seeded", "h-seeded"); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 3000; i++ {
			if err := New(root).SetPristine("/proj", "other", "h-other"); err != nil {
				t.Errorf("SetPristine() error = %v", err)
				return
			}
		}
	}()

	reader := New(root)
	for {
		select {
		case <-done:
			return
		default:
		}
		if got, ok := reader.PristineHash("/proj", "seeded"); !ok || got != "h-seeded" {
			t.Fatalf("PristineHash() = (%q, %v) while another write was in flight, want the seeded baseline intact", got, ok)
		}
	}
}

// O PristineHash devolve ok=false para um arquivo ilegível — o mesmo que "nunca
// gravei" —, então quem o chama não sabe que perdeu as linhas-base. O Verify é
// a pergunta que separa os dois casos, feita antes de qualquer efeito.
func TestVerifyTellsAnUnreadablePristineFileFromAMissingOne(t *testing.T) {
	t.Run("missing is fine", func(t *testing.T) {
		if err := New(t.TempDir()).Verify(); err != nil {
			t.Errorf("Verify() = %v, want nil when nothing was ever written", err)
		}
	})
	t.Run("valid is fine", func(t *testing.T) {
		s := New(t.TempDir())
		if err := s.SetPristine("/proj", "c", "h"); err != nil {
			t.Fatal(err)
		}
		if err := s.Verify(); err != nil {
			t.Errorf("Verify() = %v, want nil for a readable file", err)
		}
	})
	t.Run("corrupt is an error that says what to do", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "pristine.yaml")
		if err := os.WriteFile(path, []byte("{{{ not yaml"), 0o644); err != nil {
			t.Fatal(err)
		}
		err := New(root).Verify()
		if err == nil {
			t.Fatal("Verify() = nil, want an error for a corrupt pristine.yaml")
		}
		for _, want := range []string{path, "delete"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error = %q, want it to mention %q", err, want)
			}
		}
	})
}

// ---- Baseline: a linha-base que mora no projeto -------------------------

func TestBaselineRoundTripsInsideTheProject(t *testing.T) {
	target := t.TempDir()
	b := ProjectBaseline(target)

	if _, ok := b.PristineHash("tdd"); ok {
		t.Fatal("PristineHash() ok = true before anything was written")
	}
	if err := b.SetPristine("tdd", "h-tdd"); err != nil {
		t.Fatalf("SetPristine() error = %v", err)
	}
	if err := b.SetPristine("review", "h-review"); err != nil {
		t.Fatalf("SetPristine() error = %v", err)
	}

	// Outro objeto sobre o mesmo projeto enxerga o que foi gravado: o estado
	// está no arquivo, não na memória.
	got, ok := ProjectBaseline(target).PristineHash("tdd")
	if !ok || got != "h-tdd" {
		t.Errorf("PristineHash(tdd) = (%q, %v), want (h-tdd, true)", got, ok)
	}
	if _, err := os.Stat(filepath.Join(target, ".claude", ".ray-pristine.yaml")); err != nil {
		t.Errorf("the baseline file is not where the project versions it: %v", err)
	}
}

// O arquivo é commitado: chaves em ordem (diff estável) e regravar o mesmo valor
// não muda um byte (nem o mtime que acorda quem observa o diretório).
func TestBaselineFileIsStableAcrossWrites(t *testing.T) {
	target := t.TempDir()
	b := ProjectBaseline(target)
	for _, c := range []string{"zeta", "alpha", "mid"} {
		if err := b.SetPristine(c, "h-"+c); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(target, ".claude", ".ray-pristine.yaml")
	first, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(first)
	if !(strings.Index(text, "alpha") < strings.Index(text, "mid") && strings.Index(text, "mid") < strings.Index(text, "zeta")) {
		t.Errorf("keys are not sorted:\n%s", text)
	}

	old := time.Now().Add(-48 * time.Hour).Truncate(time.Second)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	if err := b.SetPristine("mid", "h-mid"); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(path)
	if string(second) != text {
		t.Errorf("rewriting the same value changed the file:\n%s", second)
	}
	if info, _ := os.Stat(path); !info.ModTime().Equal(old) {
		t.Errorf("mtime = %v, want it untouched (%v): an unchanged baseline must not be rewritten", info.ModTime(), old)
	}
}

func TestBaselineVerifyTellsUnreadableFromMissing(t *testing.T) {
	t.Run("missing is fine", func(t *testing.T) {
		if err := ProjectBaseline(t.TempDir()).Verify(); err != nil {
			t.Errorf("Verify() = %v, want nil", err)
		}
	})
	t.Run("corrupt names the project file and how to start over", func(t *testing.T) {
		target := t.TempDir()
		path := filepath.Join(target, ".claude", ".ray-pristine.yaml")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{{{ not yaml"), 0o644); err != nil {
			t.Fatal(err)
		}
		err := ProjectBaseline(target).Verify()
		if err == nil {
			t.Fatal("Verify() = nil, want an error")
		}
		for _, want := range []string{path, "delete"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error = %q, want it to mention %q", err, want)
			}
		}
		// E gravar em cima de um arquivo ilegível não pode "consertá-lo" em
		// silêncio, perdendo o que havia.
		if err := ProjectBaseline(target).SetPristine("c", "h"); err == nil {
			t.Error("SetPristine() over an unreadable file = nil error, want it refused")
		}
	})
}
