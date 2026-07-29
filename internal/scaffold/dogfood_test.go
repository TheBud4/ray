package scaffold

import (
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/profile"
)

// rayOwnHooks são os hooks que o próprio ray instala em si: os de build, porque
// o ray não roda em modo learn. Derivar de SystemFiles em vez de listar à mão
// faz um hook novo entrar no gate sozinho — e mantém guard-code.sh, que é do
// overlay de learn, legitimamente fora.
func rayOwnHooks(t *testing.T) []profile.ScaffoldFile {
	t.Helper()

	var hooks []profile.ScaffoldFile
	for _, f := range SystemFiles(ModeBuild) {
		if strings.HasPrefix(f.Path, ".claude/hooks/") {
			hooks = append(hooks, f)
		}
	}
	if len(hooks) == 0 {
		t.Fatal("SystemFiles(ModeBuild) não trouxe nenhum hook")
	}
	return hooks
}

// repoRoot devolve a raiz do repositório a partir do diretório do pacote, que é
// onde `go test` roda.
func repoRoot(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("raiz do repositório não encontrada em %s: %v", root, err)
	}
	return root
}

// TestRayOwnHooksMatchTemplates fecha o gate da regra "artefatos que andam
// juntos" (docs/conventions.md): as cópias em .claude/hooks/ são regeneradas do
// template, nunca editadas à mão. O resto da suíte renderiza em t.TempDir() e
// nunca lê .claude/hooks/, então editar um template sem regenerar a cópia
// passava verde — foi exatamente o que produziu o commit bcba2cd.
func TestRayOwnHooksMatchTemplates(t *testing.T) {
	root := repoRoot(t)
	hooks := rayOwnHooks(t)

	dir := t.TempDir()
	if _, err := WriteFiles(hooks, Options{
		Target: dir,
		Data:   Data{ProjectName: "ray", Stack: "go"},
	}); err != nil {
		t.Fatal(err)
	}

	for _, h := range hooks {
		t.Run(filepath.Base(h.Path), func(t *testing.T) {
			rel := filepath.FromSlash(h.Path)

			want, err := os.ReadFile(filepath.Join(dir, rel))
			if err != nil {
				t.Fatal(err)
			}

			ownPath := filepath.Join(root, rel)
			got, err := os.ReadFile(ownPath)
			if err != nil {
				t.Fatalf("%s ausente no próprio ray: %v\nRegenere a cópia a partir do template.", h.Path, err)
			}

			if string(got) != string(want) {
				t.Errorf("%s divergiu do template %q.\n%s\nRegenere a cópia a partir do template — ela nunca é editada à mão.",
					h.Path, templateFor[h.Path], firstDiff(string(want), string(got)))
			}

			info, err := os.Stat(ownPath)
			if err != nil {
				t.Fatal(err)
			}
			if perm := info.Mode().Perm(); perm != 0o755 {
				t.Errorf("%s tem modo %o, want 755 — o scaffold escreve .sh como executável", h.Path, perm)
			}
		})
	}
}

// TestRayOwnHooksHaveNoStrays garante o outro lado do gate: um .sh em
// .claude/hooks/ que o scaffold não escreve é arquivo órfão — ninguém o
// regenera e o settings.json não o invoca.
func TestRayOwnHooksHaveNoStrays(t *testing.T) {
	root := repoRoot(t)

	want := make(map[string]bool)
	for _, h := range rayOwnHooks(t) {
		want[filepath.Base(h.Path)] = true
	}

	entries, err := os.ReadDir(filepath.Join(root, ".claude", "hooks"))
	if err != nil {
		t.Fatal(err)
	}

	var strays []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sh") {
			continue
		}
		if !want[e.Name()] {
			strays = append(strays, e.Name())
		}
	}

	if len(strays) > 0 {
		sort.Strings(strays)
		t.Errorf(".claude/hooks/ tem %v, que o scaffold não escreve em modo build; remova ou acrescente a SystemFiles", strays)
	}
}

// firstDiff localiza a primeira linha divergente, para a falha apontar onde
// mexer em vez de despejar o arquivo inteiro.
func firstDiff(want, got string) string {
	wantLines := strings.Split(want, "\n")
	gotLines := strings.Split(got, "\n")

	for i := 0; i < len(wantLines) || i < len(gotLines); i++ {
		var w, g string
		if i < len(wantLines) {
			w = wantLines[i]
		}
		if i < len(gotLines) {
			g = gotLines[i]
		}
		if w != g {
			return "primeira divergência na linha " + strconv.Itoa(i+1) + ":\n  template: " + w + "\n  cópia:    " + g
		}
	}
	return "conteúdo igual linha a linha (divergência no final do arquivo)"
}
