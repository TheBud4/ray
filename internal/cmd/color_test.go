package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/ui"
)

const esc = "\x1b"

func noEnv(string) string { return "" }

// assertNoEscape falha se s tem um escape ANSI: a saída sem cor de qualquer
// comando não pode ter nenhum.
func assertNoEscape(t *testing.T, s string) {
	t.Helper()
	if strings.Contains(s, esc) {
		t.Errorf("output has an ANSI escape without color: %q", s)
	}
}

// ray roda o ray inteiro por run, com RAY_HOME isolado, e devolve o que saiu em
// cada fluxo e o código de saída.
func rayRun(t *testing.T, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	t.Setenv("RAY_HOME", t.TempDir())
	t.Setenv("RAY_BRAIN", "")
	t.Cleanup(func() { flagDryRun = false })
	var out, errb bytes.Buffer
	code = run(args, &out, &errb, noEnv)
	return out.String(), errb.String(), code
}

func TestColorFlagRejectsAnInvalidValueBeforeAnyEffect(t *testing.T) {
	stdout, stderr, code := rayRun(t, "--color", "rainbow", "doctor")
	if code != 1 {
		t.Errorf("exit = %d, want 1", code)
	}
	if stdout != "" {
		t.Errorf("doctor ran despite the invalid --color: %q", stdout)
	}
	if !strings.HasPrefix(stderr, "✗ ") || !strings.Contains(stderr, "auto, always or never") {
		t.Errorf("stderr = %q, want a ✗ line naming the accepted values", stderr)
	}
}

func TestColorFlagNeedsAValue(t *testing.T) {
	_, stderr, code := rayRun(t, "--color")
	if code != 1 || !strings.Contains(stderr, "needs an argument") {
		t.Errorf("exit = %d, stderr = %q; want a failure saying the flag needs an argument", code, stderr)
	}
}

// Erro de flag, de argumento e de subcomando desconhecido saem antes do
// PersistentPreRunE; a linha de erro tem de existir mesmo assim.
func TestEveryKindOfErrorGetsTheErrorLine(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"unknown flag", []string{"--nope"}},
		{"missing required argument", []string{"template", "edit"}},
		{"unknown subcommand", []string{"profile", "lst"}},
		{"runtime error", []string{"template", "show", "../x"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			stdout, stderr, code := rayRun(t, c.args...)
			assertNoEscape(t, stdout)
			if code != 1 || stdout != "" {
				t.Errorf("exit = %d, stdout = %q; want exit 1 and nothing on stdout", code, stdout)
			}
			if !strings.HasPrefix(stderr, "✗ ") || strings.Contains(stderr, "error:") || strings.Contains(stderr, esc) {
				t.Errorf("stderr = %q, want a plain ✗ line (no \"error:\" prefix, no color)", stderr)
			}
			if strings.HasSuffix(stderr, "\n\n") {
				t.Errorf("stderr = %q, want the trailing newline trimmed", stderr)
			}
			// Com cor forçada, a primeira linha fica vermelha.
			_, colored, _ := rayRun(t, append([]string{"--color", "always"}, c.args...)...)
			if !strings.HasPrefix(colored, esc+"[31m✗ ") {
				t.Errorf("with --color always, stderr = %q, want a red first line", colored)
			}
		})
	}
}

// O erro da própria flag --color não pode quebrar a linha de erro.
func TestErrorLineWorksWhenTheFlagItselfFailsToParse(t *testing.T) {
	_, stderr, code := rayRun(t, "--color", "always", "--nope")
	if code != 1 || !strings.HasPrefix(stderr, esc+"[31m✗ ") {
		t.Errorf("exit = %d, stderr = %q; --color always was parsed before --nope failed, so the line is red", code, stderr)
	}
}

// probe é um comando só de teste que usa o Printer como os comandos reais vão
// usar: ui.For(cmd.OutOrStdout()).
func probeRoot(getenv func(string) string) *cobra.Command {
	root := newRootCmdWith(getenv)
	root.AddCommand(&cobra.Command{
		Use:  "probe",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			ui.For(cmd.OutOrStdout()).Section("Components")
			ui.For(cmd.ErrOrStderr()).Section("Errors")
			return nil
		},
	})
	return root
}

func TestRootWrapsABufferOnlyWithColorAlways(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{[]string{"probe"}, false},
		{[]string{"--color", "auto", "probe"}, false},
		{[]string{"--color", "never", "probe"}, false},
		{[]string{"--color", "always", "probe"}, true},
	}
	for _, c := range cases {
		var out, errb bytes.Buffer
		root := probeRoot(noEnv)
		root.SetOut(&out)
		root.SetErr(&errb)
		root.SetArgs(c.args)
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if got := strings.Contains(out.String(), esc); got != c.want {
			t.Errorf("%v: stdout has escape = %v, want %v (%q)", c.args, got, c.want, out.String())
		}
		if got := strings.Contains(errb.String(), esc); got != c.want {
			t.Errorf("%v: stderr has escape = %v, want %v (%q)", c.args, got, c.want, errb.String())
		}
	}
}

// Nenhum subcomando declara o próprio PersistentPreRun(E): ele desligaria o da
// raiz em silêncio (o cobra só roda o mais próximo).
func TestNoSubcommandDeclaresPersistentPreRun(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			if sub.PersistentPreRun != nil || sub.PersistentPreRunE != nil {
				t.Errorf("%q declares its own PersistentPreRun(E), which would silently disable the root's", sub.CommandPath())
			}
			walk(sub)
		}
	}
	walk(newRootCmd())
}

// O writer que vai ao processo filho tem de ser o *os.File de verdade: com um
// Printer no meio, o Go criaria um pipe no lugar do terminal.
func TestRunGivesTheChildTheRawWriters(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ray.yaml"), []byte("commands:\n  hello:\n    steps: [\"echo hi\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	t.Setenv("RAY_HOME", t.TempDir())

	fake := &runner.FakeRunner{}
	prev := newRunRunner
	newRunRunner = func(bool, io.Writer) runner.Runner { return fake }
	t.Cleanup(func() { newRunRunner = prev })

	stdout, err := os.Create(filepath.Join(dir, "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	defer stdout.Close()
	stderr, err := os.Create(filepath.Join(dir, "stderr"))
	if err != nil {
		t.Fatal(err)
	}
	defer stderr.Close()

	// --color always embrulha os dois fluxos em Printers; o filho não pode ver isso.
	if code := run([]string{"--color", "always", "run", "hello"}, stdout, stderr, noEnv); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if len(fake.Calls) != 1 {
		t.Fatalf("runner calls = %d, want 1", len(fake.Calls))
	}
	c := fake.Calls[0]
	if c.Stdout != io.Writer(stdout) || c.Stderr != io.Writer(stderr) {
		t.Errorf("the child got Stdout %T / Stderr %T, want the raw *os.File values", c.Stdout, c.Stderr)
	}
}

// Com --color always, o que não é saída semântica sai byte a byte como sem a
// flag: o conteúdo de template, os caminhos feitos para $(…), o help, a versão
// e a conclusão.
func TestSemanticColorNeverTouchesRawOutput(t *testing.T) {
	cases := [][]string{
		{"template", "show", "CLAUDE.md.tmpl"},
		{"template", "path"},
		{"template", "path", "CLAUDE.md.tmpl"},
		{"profile", "path"},
		{"brain", "path"},
		{"--help"},
		{"profile", "--help"},
		{"--version"},
		{"completion", "bash"},
		{"__complete", "template", "show", ""},
	}
	brain := t.TempDir() // um cérebro configurado: `brain path` imprime o caminho em vez de falhar
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			home := t.TempDir()
			plain := func(extra ...string) (string, string) {
				t.Setenv("RAY_HOME", home)
				t.Setenv("RAY_BRAIN", brain)
				var out, errb bytes.Buffer
				run(append(extra, args...), &out, &errb, noEnv)
				return out.String(), errb.String()
			}
			wantOut, wantErr := plain()
			gotOut, gotErr := plain("--color", "always")
			if gotOut != wantOut || gotErr != wantErr {
				t.Errorf("--color always changed the output of %v:\nstdout %q\nvs     %q\nstderr %q\nvs     %q", args, gotOut, wantOut, gotErr, wantErr)
			}
		})
	}
}

// stripEscapes tira as sequências SGR de s: o que sobra é o texto que um
// terminal sem cor mostraria.
func stripEscapes(s string) string {
	return sgrPattern.ReplaceAllString(s, "")
}

var sgrPattern = regexp.MustCompile("\x1b\\[[0-9;]*m")
