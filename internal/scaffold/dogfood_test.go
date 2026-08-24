package scaffold

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/profile"
)

// rayOwnHooks são os hooks que o próprio ray instala em si. Derivar de
// SystemFiles em vez de listar à mão faz um hook novo entrar no gate sozinho.
func rayOwnHooks(t *testing.T) []profile.ScaffoldFile {
	t.Helper()

	var hooks []profile.ScaffoldFile
	for _, f := range SystemFiles() {
		if strings.HasPrefix(f.Path, ".claude/hooks/") {
			hooks = append(hooks, f)
		}
	}
	if len(hooks) == 0 {
		t.Fatal("SystemFiles() returned no hooks")
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
		t.Fatalf("repository root not found at %s: %v", root, err)
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
				t.Fatalf("%s missing from ray itself: %v\nRegenerate the copy from the template.", h.Path, err)
			}

			if string(got) != string(want) {
				t.Errorf("%s diverged from template %q.\n%s\nRegenerate the copy from the template; it is never hand-edited.",
					h.Path, templateFor[h.Path], firstDiff(string(want), string(got)))
			}

			info, err := os.Stat(ownPath)
			if err != nil {
				t.Fatal(err)
			}
			if perm := info.Mode().Perm(); perm != 0o755 {
				t.Errorf("%s has mode %o, want 755: scaffold writes .sh as executable", h.Path, perm)
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
		t.Errorf(".claude/hooks/ has %v, which scaffold does not write in build mode; remove it or add it to SystemFiles", strays)
	}
}

// TestRayOwnSettingsMatchHookSettings fecha o outro lado do gate de dogfood. O
// ray declara a mesma fiação de hooks em dois lugares: HookSettings, que a
// escreve nos projetos dos outros, e .claude/settings.json, que ele usa em si.
// Só o primeiro era testado, e o segundo derivou — os matchers do guard-plans e
// do guard-vocab ficaram para trás quando o MultiEdit entrou.
//
// TestFileEditingHooksCoverMultiEdit existe justamente para impedir esse
// buraco, e passava verde o tempo todo: ele olha o que o HookSettings gera,
// nunca o arquivo que o ray usa. Uma asserção sobre o gerador não diz nada
// sobre a cópia que ninguém regenera.
func TestRayOwnSettingsMatchHookSettings(t *testing.T) {
	root := repoRoot(t)

	raw, err := os.ReadFile(filepath.Join(root, ".claude", "settings.json"))
	if err != nil {
		t.Fatal(err)
	}

	var own map[string]any
	if err := json.Unmarshal(raw, &own); err != nil {
		t.Fatalf(".claude/settings.json is not valid JSON: %v", err)
	}

	want := HookSettings()["hooks"]
	got := own["hooks"]

	// DeepEqual e não comparação de texto: o arquivo é JSON escrito à mão e
	// alvo de merge, então indentação e ordem de chave não são contrato. A
	// ordem *dentro* de cada evento é: ela decide qual aviso sai primeiro.
	if !reflect.DeepEqual(got, want) {
		t.Errorf(".claude/settings.json diverged from HookSettings().\ngot:\n%s\nwant:\n%s\nThe two declare the same wiring; align the file by hand.",
			indentJSON(t, got), indentJSON(t, want))
	}
}

// indentJSON formata um lado da comparação para a falha ser legível em vez de
// um despejo de map[string]interface {} numa linha só.
func indentJSON(t *testing.T, v any) []byte {
	t.Helper()

	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return b
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
