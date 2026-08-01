package scaffold

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemFilesBuildIsSessionStartGuardAddGuardVocabAndGuardPlans(t *testing.T) {
	files := SystemFiles(ModeBuild)
	want := []string{".claude/hooks/session-start.sh", ".claude/hooks/guard-add.sh", ".claude/hooks/guard-vocab.sh", ".claude/hooks/guard-plans.sh"}
	if len(files) != len(want) {
		t.Fatalf("SystemFiles(build) = %v, want %v", files, want)
	}
	for i, p := range want {
		if files[i].Path != p {
			t.Fatalf("SystemFiles(build)[%d] = %q, want %q", i, files[i].Path, p)
		}
	}
}

func TestSystemFilesLearnAddsGuardAndRule(t *testing.T) {
	files := SystemFiles(ModeLearn)
	want := map[string]bool{
		".claude/hooks/session-start.sh":    false,
		".claude/hooks/guard-add.sh":        false,
		".claude/hooks/guard-vocab.sh":      false,
		".claude/hooks/guard-plans.sh":      false,
		".claude/rules/learn.md":            false,
		".claude/hooks/guard-code.sh":       false,
		".claude/rules/learning-journal.md": false,
		".claude/rules/learn-teaching.md":   false,
	}
	if len(files) != len(want) {
		t.Fatalf("SystemFiles(learn) = %v, want %d entries", files, len(want))
	}
	for _, f := range files {
		if _, ok := want[f.Path]; !ok {
			t.Fatalf("unexpected path %q in SystemFiles(learn)", f.Path)
		}
		want[f.Path] = true
	}
	for p, seen := range want {
		if !seen {
			t.Fatalf("SystemFiles(learn) missing %q", p)
		}
	}
}

func TestLearnOverlayWritesTeachingPrompt(t *testing.T) {
	build := t.TempDir()
	if _, err := WriteFiles(SystemFiles(ModeBuild), Options{Target: build}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(build, ".claude/rules/learn-teaching.md")); !os.IsNotExist(err) {
		t.Error("build mode wrote the teaching prompt; it belongs to the learn overlay")
	}

	learn := t.TempDir()
	if _, err := WriteFiles(SystemFiles(ModeLearn), Options{Target: learn}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(learn, ".claude/rules/learn-teaching.md"))
	if err != nil {
		t.Fatalf("teaching prompt was not written: %v", err)
	}

	// Os pilares do redesenho, mais o exemplo de milestones.yaml. Se um pilar
	// sumir, o modo volta a ser proibição sem pedagogia.
	//
	// As âncoras são *proposições*, não vocabulário: "escada" e "fatos" — que
	// estavam aqui antes — casariam quase qualquer prosa sobre ensino e
	// passariam mesmo com o pilar arrancado. Cada frase abaixo só existe se a
	// regra que ela enuncia existir. A última é o bloco YAML de exemplo: se
	// ele sumir ou corromper, a IA grava um milestones.yaml que
	// profile.ValidateMilestones recusa.
	for _, want := range []string{
		"Fatos se respondem direto",            // fato não entra na escada
		"O aluno puxa",                         // quem controla o degrau
		"O degrau reseta a cada problema novo", // impede a catraca até o topo
		"Você sobe sozinha só com evidência",   // a exceção ao "aluno puxa"
		"o que você já tentou",                 // o portão do degrau 4
		"npm test -- tasks.e2e",                // exemplo de milestones.yaml
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("teaching prompt does not mention %q", want)
		}
	}
}

func TestHookSettingsBuildHasThreeWarningGuardsInPreToolUse(t *testing.T) {
	settings := HookSettings(ModeBuild)
	hooks, ok := settings["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("hooks missing or wrong type: %#v", settings)
	}
	if _, ok := hooks["SessionStart"]; !ok {
		t.Fatalf("hooks = %#v, want SessionStart", hooks)
	}
	pre, ok := hooks["PreToolUse"].([]any)
	if !ok {
		t.Fatalf("hooks = %#v, want PreToolUse in build mode (the three warning guards are unconditional)", hooks)
	}
	if len(pre) != 3 {
		t.Fatalf("PreToolUse = %#v, want guard-add, guard-plans and guard-vocab in build mode", pre)
	}
	first, ok := pre[0].(map[string]any)
	if !ok || first["matcher"] != "Bash" {
		t.Fatalf("PreToolUse[0] = %#v, want matcher \"Bash\" (guard-add.sh)", pre[0])
	}
	second, ok := pre[1].(map[string]any)
	if !ok || second["matcher"] != "Edit|Write|MultiEdit" {
		t.Fatalf("PreToolUse[1] = %#v, want matcher \"Edit|Write|MultiEdit\" (guard-plans.sh)", pre[1])
	}
	third, ok := pre[2].(map[string]any)
	if !ok || third["matcher"] != "Edit|Write|MultiEdit" {
		t.Fatalf("PreToolUse[2] = %#v, want matcher \"Edit|Write|MultiEdit\" (guard-vocab.sh)", pre[2])
	}

	// guard-vocab lê o payload, não o disco: não há mais razão para rodar
	// depois da escrita, e avisar depois não redireciona nada. Sem esta
	// asserção, deixar o hook declarado nos dois eventos passaria verde — e o
	// aviso sairia em dobro.
	if _, ok := hooks["PostToolUse"]; ok {
		t.Errorf("hooks still declare PostToolUse: %#v", hooks["PostToolUse"])
	}
}

func TestHookSettingsLearnHasPreToolUse(t *testing.T) {
	settings := HookSettings(ModeLearn)
	hooks := settings["hooks"].(map[string]any)
	if _, ok := hooks["SessionStart"]; !ok {
		t.Fatalf("hooks = %#v, want SessionStart", hooks)
	}
	pre, ok := hooks["PreToolUse"].([]any)
	if !ok {
		t.Fatalf("hooks = %#v, want PreToolUse in learn mode", hooks)
	}
	if len(pre) != 4 {
		t.Fatalf("PreToolUse = %#v, want guard-add (Bash) plus guard-plans, guard-vocab and guard-code (Edit|Write|MultiEdit)", pre)
	}
	first, ok := pre[0].(map[string]any)
	if !ok || first["matcher"] != "Bash" {
		t.Fatalf("PreToolUse[0] = %#v, want matcher \"Bash\" (guard-add.sh)", pre[0])
	}
	second, ok := pre[1].(map[string]any)
	if !ok || second["matcher"] != "Edit|Write|MultiEdit" {
		t.Fatalf("PreToolUse[1] = %#v, want matcher \"Edit|Write|MultiEdit\" (guard-plans.sh)", pre[1])
	}
	third, ok := pre[2].(map[string]any)
	if !ok || third["matcher"] != "Edit|Write|MultiEdit" {
		t.Fatalf("PreToolUse[2] = %#v, want matcher \"Edit|Write|MultiEdit\" (guard-vocab.sh)", pre[2])
	}
	// O guard-code é do overlay de learn e entra por último, depois dos três
	// guards de aviso que os dois modos compartilham.
	fourth, ok := pre[3].(map[string]any)
	if !ok || fourth["matcher"] != "Edit|Write|MultiEdit" {
		t.Fatalf("PreToolUse[3] = %#v, want matcher \"Edit|Write|MultiEdit\" (guard-code.sh)", pre[3])
	}
}

func requireBashAndJQ(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not available")
	}
	if _, err := exec.LookPath("jq"); err != nil {
		t.Skip("jq not available")
	}
}

func runGuardCode(t *testing.T, scriptPath, filePath string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"tool_input": map[string]any{"file_path": filePath},
	})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", scriptPath)
	cmd.Stdin = bytes.NewReader(payload)
	// Hook é invocado da raiz do projeto; PWD usado para cálculo de caminho relativo
	// deve ser a raiz, não o subdiretório .claude/hooks.
	cmd.Dir = filepath.Join(filepath.Dir(scriptPath), "..", "..")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("guard-code.sh failed: %v\noutput: %s", err, out.String())
	}
	return out.String()
}

// requireDeny trava a *forma* da negação, não só o fato dela. Em PreToolUse a
// doc do Claude Code manda usar hookSpecificOutput.permissionDecision; o
// {"decision":"block"} de topo ainda é honrado (verificado em 2.1.220) mas é
// desaconselhado, e este hook é o único bloqueio do modo learn. Um teste que só
// procurasse a substring "block" passaria com a forma legada de volta.
func requireDeny(t *testing.T, out, ctx string) {
	t.Helper()
	var got struct {
		Decision           string `json:"decision"`
		HookSpecificOutput struct {
			HookEventName            string `json:"hookEventName"`
			PermissionDecision       string `json:"permissionDecision"`
			PermissionDecisionReason string `json:"permissionDecisionReason"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("%s: output is not valid JSON (%v): %q", ctx, err, out)
	}
	if got.Decision != "" {
		t.Errorf("%s: uses the legacy top-level \"decision\" field, discouraged for PreToolUse: %q", ctx, out)
	}
	if got.HookSpecificOutput.HookEventName != "PreToolUse" {
		t.Errorf("%s: hookEventName = %q, want \"PreToolUse\"", ctx, got.HookSpecificOutput.HookEventName)
	}
	if got.HookSpecificOutput.PermissionDecision != "deny" {
		t.Fatalf("%s: permissionDecision = %q, want \"deny\": the learn-mode floor is not denying", ctx, got.HookSpecificOutput.PermissionDecision)
	}
	if strings.TrimSpace(got.HookSpecificOutput.PermissionDecisionReason) == "" {
		t.Errorf("%s: denial without a reason: the assistant has nothing to explain to the student", ctx)
	}
}

func TestGuardCodeAllowsDocsBlocksCode(t *testing.T) {
	requireBashAndJQ(t)

	target := t.TempDir()
	if _, err := WriteFiles(SystemFiles(ModeLearn), Options{Target: target}); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(target, ".claude/hooks/guard-code.sh")

	if err := os.MkdirAll(filepath.Join(target, "docs"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(target, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}

	docsOut := runGuardCode(t, scriptPath, filepath.Join(target, "docs/x.md"))
	if strings.TrimSpace(docsOut) != "" {
		t.Fatalf("guard-code.sh on docs/x.md = %q, want empty (allowed)", docsOut)
	}

	codeOut := runGuardCode(t, scriptPath, filepath.Join(target, "lib/main.dart"))
	requireDeny(t, codeOut, "lib/main.dart")
}

// Um caminho com aspas ou barra invertida já quebrou JSON montado à mão. JSON
// inválido num hook que nega é falha *aberta*: o Claude Code não lê uma decisão
// e a edição passa. Por isso a negação é montada com jq, e por isso isto é teste.
func TestGuardCodeDenialIsValidJSONForHostilePaths(t *testing.T) {
	requireBashAndJQ(t)

	target := t.TempDir()
	if _, err := WriteFiles(SystemFiles(ModeLearn), Options{Target: target}); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(target, ".claude/hooks/guard-code.sh")

	for _, name := range []string{`lib/a"b.go`, `lib/a\b.go`, "lib/a\tb.go"} {
		out := runGuardCode(t, scriptPath, filepath.Join(target, name))
		requireDeny(t, out, name)
	}
}

// runGuardCodeWithoutJQ roda o hook com PATH vazio. Como a checagem de jq usa
// só builtins de bash (`command`, `echo`), o hook consegue decidir mesmo sem
// nenhum binário externo disponível.
func runGuardCodeWithoutJQ(t *testing.T, scriptPath, filePath string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"tool_input": map[string]any{"file_path": filePath},
	})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", scriptPath)
	cmd.Stdin = bytes.NewReader(payload)
	// Hook é invocado da raiz do projeto; PWD usado para cálculo de caminho relativo
	// deve ser a raiz, não o subdiretório .claude/hooks.
	cmd.Dir = filepath.Join(filepath.Dir(scriptPath), "..", "..")
	cmd.Env = []string{"PATH="}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("guard-code.sh without jq failed: %v\noutput: %s", err, out.String())
	}
	return out.String()
}

func TestGuardCodeBlocksWhenJQMissing(t *testing.T) {
	requireBashAndJQ(t)

	target := t.TempDir()
	if _, err := WriteFiles(SystemFiles(ModeLearn), Options{Target: target}); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(target, ".claude/hooks/guard-code.sh")

	// Um .md é permitido quando jq existe; sem jq o hook não tem como saber
	// disso, e a resposta certa é bloquear, não deixar passar.
	out := runGuardCodeWithoutJQ(t, scriptPath, filepath.Join(target, "docs/x.md"))
	requireDeny(t, out, "without jq")
	if !strings.Contains(out, "jq") {
		t.Errorf("the message does not say jq is missing: %q", out)
	}
}

// runGuardCodeIn roda o hook com um cwd e um $PWD escolhidos separadamente — é
// o que permite simular projeto alcançado por symlink, onde o bash herda o
// caminho lógico e `pwd -P` devolve o físico.
func runGuardCodeIn(t *testing.T, scriptPath, dir, pwd, filePath string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"tool_input": map[string]any{"file_path": filePath},
	})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", scriptPath)
	cmd.Stdin = bytes.NewReader(payload)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PWD="+pwd)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("guard-code.sh failed: %v\noutput: %s", err, out.String())
	}
	return out.String()
}

// O strip de prefixo tem de casar tanto quando o projeto é alcançado pelo
// caminho lógico quanto pelo físico. Se só uma forma fosse tentada, escrita
// legítima em .claude/** viraria falso bloqueio — e .claude/.local/ é o fluxo
// principal do modo learn (milestones.yaml, diário, rascunho/).
func TestGuardCodeAllowsClaudeDirReachedThroughSymlink(t *testing.T) {
	requireBashAndJQ(t)

	root := t.TempDir()
	physical := filepath.Join(root, "fisico")
	if err := os.MkdirAll(physical, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := WriteFiles(SystemFiles(ModeLearn), Options{Target: physical}); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(physical, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	scriptPath := filepath.Join(physical, ".claude/hooks/guard-code.sh")

	// t.TempDir() pode conter symlink por si (macOS: /var → /private/var), então
	// a forma física é derivada, não presumida.
	realRoot, err := filepath.EvalSymlinks(physical)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name     string
		pwd      string
		filePath string
	}{
		{"logical pwd, physical file_path", link, filepath.Join(realRoot, ".claude/.local/milestones.yaml")},
		{"logical pwd, logical file_path", link, filepath.Join(link, ".claude/.local/milestones.yaml")},
		{"physical pwd, physical file_path", realRoot, filepath.Join(realRoot, ".claude/.local/milestones.yaml")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := runGuardCodeIn(t, scriptPath, link, tc.pwd, tc.filePath)
			if strings.TrimSpace(out) != "" {
				t.Errorf("guard-code denied a legitimate write to .claude/.local/: %q", out)
			}
		})
	}

	// O contrário segue valendo: código continua negado por qualquer das formas.
	out := runGuardCodeIn(t, scriptPath, link, link, filepath.Join(realRoot, "main.go"))
	requireDeny(t, out, "main.go through symlink")
}

func TestGuardCodeAllowsTheScratchDir(t *testing.T) {
	requireBashAndJQ(t)

	target := t.TempDir()
	if _, err := WriteFiles(SystemFiles(ModeLearn), Options{Target: target}); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(target, ".claude/hooks/guard-code.sh")

	// .claude/.local/rascunho/ é onde a IA escreve demonstração rodável: já é
	// permitido pelo padrão .claude/* e já é gitignorado pelo I1.
	out := runGuardCode(t, scriptPath, filepath.Join(target, ".claude/.local/rascunho/demo.go"))
	if strings.TrimSpace(out) != "" {
		t.Errorf("guard-code bloqueou o rascunho: %q", out)
	}
}

func runSessionStart(t *testing.T, scriptPath string) string {
	t.Helper()
	cmd := exec.Command("bash", scriptPath)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("session-start.sh failed: %v\noutput: %s", err, out.String())
	}
	return out.String()
}

func TestSessionStartCountsInjectedHandoffs(t *testing.T) {
	requireBashAndJQ(t)

	target := t.TempDir()
	if _, err := WriteFiles(SystemFiles(ModeBuild), Options{Target: target}); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(target, ".claude/hooks/session-start.sh")
	countPath := filepath.Join(target, ".claude/.ray-metrics/handoffs.count")

	if err := os.WriteFile(filepath.Join(target, ".claude/handoff.md"), []byte("state"), 0o644); err != nil {
		t.Fatal(err)
	}

	out1 := runSessionStart(t, scriptPath)
	if !strings.Contains(out1, "state") {
		t.Fatalf("session-start.sh output = %q, want it to include the handoff content", out1)
	}
	data, err := os.ReadFile(countPath)
	if err != nil {
		t.Fatalf("stat handoffs.count after 1st run: %v", err)
	}
	if strings.TrimSpace(string(data)) != "1" {
		t.Errorf("handoffs.count after 1st run = %q, want %q", data, "1")
	}

	runSessionStart(t, scriptPath)
	data, err = os.ReadFile(countPath)
	if err != nil {
		t.Fatalf("stat handoffs.count after 2nd run: %v", err)
	}
	if strings.TrimSpace(string(data)) != "2" {
		t.Errorf("handoffs.count after 2nd run = %q, want %q", data, "2")
	}
}

func TestSessionStartInjectsJournalHeadWhenPresent(t *testing.T) {
	requireBashAndJQ(t)

	target := t.TempDir()
	if _, err := WriteFiles(SystemFiles(ModeBuild), Options{Target: target}); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(target, ".claude/hooks/session-start.sh")

	journalDir := filepath.Join(target, ".claude/.local")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(journalDir, "learning-journal.md"), []byte("## Combinado\n- Degrau inicial: 2\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runSessionStart(t, scriptPath)
	if !strings.Contains(out, "## Combinado") {
		t.Errorf("session-start.sh output = %q, want it to include the journal head", out)
	}
}

func TestSessionStartInjectsMilestonesProgressWhenPresent(t *testing.T) {
	requireBashAndJQ(t)

	target := t.TempDir()
	if _, err := WriteFiles(SystemFiles(ModeBuild), Options{Target: target}); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(target, ".claude/hooks/session-start.sh")

	journalDir := filepath.Join(target, ".claude/.local")
	if err := os.MkdirAll(journalDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(journalDir, "milestones-progress.md"), []byte("Milestones passed: 1/2"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runSessionStart(t, scriptPath)
	if !strings.Contains(out, "Milestones passed: 1/2") {
		t.Errorf("session-start.sh output = %q, want it to include the milestones progress", out)
	}
}

func TestSessionStartWithoutHandoffRecordsNoMetric(t *testing.T) {
	requireBashAndJQ(t)

	target := t.TempDir()
	if _, err := WriteFiles(SystemFiles(ModeBuild), Options{Target: target}); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(target, ".claude/hooks/session-start.sh")

	out := runSessionStart(t, scriptPath)
	if strings.TrimSpace(out) != "" {
		t.Errorf("session-start.sh output = %q, want empty with no handoff.md", out)
	}
	if _, err := os.Stat(filepath.Join(target, ".claude/.ray-metrics")); !os.IsNotExist(err) {
		t.Error(".ray-metrics/ should not exist: no handoff was injected, no activity to record")
	}
}
