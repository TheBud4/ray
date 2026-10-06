package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/runfile"
	"github.com/TheBud4/ray/internal/runner"
)

func testCommands() map[string]runfile.Resolved {
	return map[string]runfile.Resolved{
		"test": {Name: "test", Description: "run tests", Steps: []string{"go test ./..."}, BaseDir: "/proj", Source: runfile.SourceProject},
		"lint": {Name: "lint", Description: "lint everything", Steps: []string{"echo one", "echo two"}, BaseDir: "/home", Source: runfile.SourceGlobal},
	}
}

func TestRunRunCmdListsWithNoAliasOrFlag(t *testing.T) {
	var out bytes.Buffer
	if err := runRunCmd(testCommands(), "", nil, false, &runner.FakeRunner{}, false, &out); err != nil {
		t.Fatalf("runRunCmd() error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "test") || !strings.Contains(got, "lint") {
		t.Errorf("output = %q, want it to list both aliases", got)
	}
	if !strings.Contains(got, runfile.SourceProject) || !strings.Contains(got, runfile.SourceGlobal) {
		t.Errorf("output = %q, want it to show each alias's source", got)
	}
}

// Sem alias, a tabela saía só com o cabeçalho — que informa tanto quanto uma
// página em branco, e ainda parece que algo deveria estar listado.
func TestPrintAliasListSaysWhenThereAreNoRuns(t *testing.T) {
	var out bytes.Buffer
	printAliasList(&out, map[string]runfile.Resolved{})

	got := out.String()
	if !strings.Contains(got, "no runs defined (add a commands: block to ray.yaml)") {
		t.Errorf("output = %q, want it to say there are no runs and where to define them", got)
	}
	if strings.Contains(got, "NAME") {
		t.Errorf("output = %q, want no table header when there is no row", got)
	}
}

// A regressão: com alias, o cabeçalho e as linhas continuam saindo.
func TestPrintAliasListStillPrintsTheTableWhenThereAreRuns(t *testing.T) {
	var out bytes.Buffer
	printAliasList(&out, testCommands())

	got := out.String()
	if !strings.Contains(got, "NAME") {
		t.Errorf("output = %q, want the table header", got)
	}
	for _, name := range []string{"test", "lint"} {
		if !strings.Contains(got, name) {
			t.Errorf("output = %q, want it to list %q", got, name)
		}
	}
	if strings.Contains(got, "no runs defined") {
		t.Errorf("output = %q, want no empty-list line when there are runs", got)
	}
}

func TestRunRunCmdUnknownAliasErrors(t *testing.T) {
	err := runRunCmd(testCommands(), "nope", nil, false, &runner.FakeRunner{}, false, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runRunCmd() = nil error, want error for an unknown alias")
	}
	if !strings.Contains(err.Error(), "--list") {
		t.Errorf("error = %q, want it to point at `ray run --list`", err.Error())
	}
}

func TestRunRunCmdAbortsOnFirstFailure(t *testing.T) {
	fr := &runner.FakeRunner{Results: map[string]runner.Result{
		"echo one": {ExitCode: 1},
	}}
	err := runRunCmd(testCommands(), "lint", nil, false, fr, false, &bytes.Buffer{})
	if err == nil {
		t.Fatal("runRunCmd() = nil error, want error when the first step fails")
	}
	if len(fr.Calls) != 1 {
		t.Fatalf("Calls = %v, want exactly 1 (aborted before the 2nd step)", fr.Calls)
	}
}

func TestRunRunCmdAppendsExtraArgsToLastStep(t *testing.T) {
	fr := &runner.FakeRunner{}
	err := runRunCmd(testCommands(), "lint", []string{"--foo"}, false, fr, false, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("runRunCmd() error = %v", err)
	}
	if len(fr.Calls) != 2 {
		t.Fatalf("Calls = %v, want 2 steps to run", fr.Calls)
	}
	if fr.Calls[0].String() != "echo one" {
		t.Errorf("first call = %q, want unchanged \"echo one\"", fr.Calls[0].String())
	}
	if fr.Calls[1].String() != "echo two --foo" {
		t.Errorf("last call = %q, want extra args appended", fr.Calls[1].String())
	}
}

func TestSplitAliasArgs(t *testing.T) {
	cases := []struct {
		name      string
		args      []string
		dashAt    int
		wantAlias string
		wantExtra []string
	}{
		{"no args", nil, -1, "", nil},
		{"alias only", []string{"test"}, -1, "test", nil},
		{"alias with dash", []string{"test", "-run", "TestX"}, 1, "test", []string{"-run", "TestX"}},
		{"dash with no alias", []string{"-v"}, 0, "", []string{"-v"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			alias, extra := splitAliasArgs(tc.args, tc.dashAt)
			if alias != tc.wantAlias {
				t.Errorf("alias = %q, want %q", alias, tc.wantAlias)
			}
			if len(extra) != len(tc.wantExtra) {
				t.Fatalf("extra = %v, want %v", extra, tc.wantExtra)
			}
			for i := range extra {
				if extra[i] != tc.wantExtra[i] {
					t.Errorf("extra[%d] = %q, want %q", i, extra[i], tc.wantExtra[i])
				}
			}
		})
	}
}

// Um passo é dividido como um shell divide a linha — aspas e barra invertida —
// para que `git commit -m "a b"` chegue ao git como TRÊS argumentos, não como
// `"a` e `b"`. Sem expansão de variável nem de curinga: o ray não é um shell.
func TestRunRunCmdSplitsStepsLikeAShell(t *testing.T) {
	cases := []struct {
		step string
		want []string
	}{
		{`git commit -m "a b"`, []string{"git", "commit", "-m", "a b"}},
		{`echo 'it'"'"'s'`, []string{"echo", "it's"}},
		{`echo a\ b`, []string{"echo", "a b"}},
		{`echo ""`, []string{"echo", ""}},
		{`echo "say \"hi\""`, []string{"echo", `say "hi"`}},
		{`echo '$HOME \n'`, []string{"echo", `$HOME \n`}},
		{"  go   test  ./... ", []string{"go", "test", "./..."}},
	}
	for _, tc := range cases {
		t.Run(tc.step, func(t *testing.T) {
			commands := map[string]runfile.Resolved{"x": {Name: "x", Steps: []string{tc.step}}}
			fr := &runner.FakeRunner{}
			if err := runRunCmd(commands, "x", nil, false, fr, false, &bytes.Buffer{}); err != nil {
				t.Fatalf("runRunCmd() error = %v", err)
			}
			if len(fr.Calls) != 1 {
				t.Fatalf("Calls = %v, want one", fr.Calls)
			}
			got := append([]string{fr.Calls[0].Name}, fr.Calls[0].Args...)
			if !slices.Equal(got, tc.want) {
				t.Errorf("command = %q, want %q", got, tc.want)
			}
		})
	}
}

// Os argumentos depois de `--` são acrescentados como vieram: já são argumentos
// separados, não texto a dividir de novo.
func TestRunRunCmdKeepsExtraArgsWholeAfterSplitting(t *testing.T) {
	commands := map[string]runfile.Resolved{"x": {Name: "x", Steps: []string{`echo "a b"`}}}
	fr := &runner.FakeRunner{}
	if err := runRunCmd(commands, "x", []string{"c d"}, false, fr, false, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if got := fr.Calls[0].Args; !slices.Equal(got, []string{"a b", "c d"}) {
		t.Errorf("Args = %q, want [\"a b\" \"c d\"]", got)
	}
}

// Aspas sem fechar são um erro do ray.yaml. E nenhum passo roda: se o segundo
// passo é inválido, o primeiro não pode já ter sido executado.
func TestRunRunCmdRefusesAnUnterminatedQuoteBeforeRunningAnything(t *testing.T) {
	commands := map[string]runfile.Resolved{"x": {Name: "x", Steps: []string{"echo ok", `echo "abc`}}}
	fr := &runner.FakeRunner{}

	err := runRunCmd(commands, "x", nil, false, fr, false, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "unterminated") || !strings.Contains(err.Error(), `echo "abc`) {
		t.Fatalf("error = %v, want one naming the unterminated quote and the step", err)
	}
	if len(fr.Calls) != 0 {
		t.Errorf("Calls = %v, want nothing run when any step is invalid", fr.Calls)
	}
}

func rayYAMLWithHello(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ray.yaml"), []byte("commands:\n  hello:\n    steps:\n      - echo oi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
}

// `ray run alias x y` sem `--` descartava `x y` e saía 0, como se a pessoa
// tivesse pedido outra coisa. Agora recusa e diz onde pôr os argumentos.
func TestRunRefusesStrayArgumentsWithoutTheDash(t *testing.T) {
	dryRunHome(t)
	rayYAMLWithHello(t)

	out, err := execRoot(t, "run", "--dry-run", "hello", "x", "y")
	if err == nil || !strings.Contains(err.Error(), "unexpected argument") || !strings.Contains(err.Error(), "--") {
		t.Fatalf("Execute() error = %v, want an error about unexpected arguments that mentions `--`", err)
	}
	if strings.Contains(out, "+ echo") {
		t.Errorf("output = %q, the alias must not run when arguments were dropped", out)
	}
}

// Controle: o caminho certo continua funcionando.
func TestRunStillForwardsArgumentsAfterTheDash(t *testing.T) {
	dryRunHome(t)
	rayYAMLWithHello(t)

	out, err := execRoot(t, "run", "--dry-run", "hello", "--", "x", "y")
	if err != nil {
		t.Fatalf("Execute() error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "+ echo oi x y") {
		t.Errorf("output = %q, want the extra args forwarded", out)
	}
}
