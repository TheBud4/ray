package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// O Cobra toma `-v` para `--version` quando encontra o atalho livre, e o
// resultado é pior que um atalho trocado: `ray -v <comando>` imprime a versão,
// não roda o comando e sai 0. Quem declara `--verbose` primeiro fica com o
// atalho, e a flag de versão nasce só na forma longa.
func TestVerboseOwnsTheVShorthand(t *testing.T) {
	root := newRootCmd()
	// O Cobra só registra a flag de versão dentro do Execute; chamar aqui é o
	// que faz o teste enxergar a árvore de flags como o binário a monta.
	root.InitDefaultVersionFlag()

	sh := root.Flags().ShorthandLookup("v")
	if sh == nil {
		t.Fatal(`no flag owns the "v" shorthand`)
	}
	if sh.Name != "verbose" {
		t.Errorf(`-v is the shorthand of %q, want "verbose"`, sh.Name)
	}
	if root.Flags().Lookup("version") == nil {
		t.Error("--version is gone; it must survive in its long form")
	}
}

// Comando que só agrupa não roda nada, e o Cobra responde a um filho
// inexistente imprimindo o help e devolvendo nil — ou seja, `ray profile lst`
// saía 0 sem ter feito nada. A varredura é da árvore inteira de propósito: um
// grupo novo não pode nascer com o defeito.
func TestGroupCommandsRejectUnknownSubcommand(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if c.HasSubCommands() {
			if err := c.ValidateArgs([]string{"bogus"}); err == nil {
				t.Errorf("%q accepts an unknown subcommand; want it rejected", c.CommandPath())
			}
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(newRootCmd())
}

// A consequência, pelo caminho real: o Execute tem de devolver erro, que é o
// que vira exit 1 no Execute() do pacote.
func TestUnknownSubcommandIsAnError(t *testing.T) {
	t.Setenv("RAY_HOME", t.TempDir())

	var out bytes.Buffer
	root := newRootCmd()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"profile", "lst"})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() = nil, want an error for an unknown subcommand")
	}
	if !strings.Contains(err.Error(), "lst") {
		t.Errorf("error = %q, want it to name the unknown subcommand", err)
	}
}

// A consequência que motiva o CA: com `-v` na frente, o subcomando tem de
// rodar. Um exit 0 sem efeito é o modo de falha perigoso — em script, passa
// por sucesso.
func TestVShorthandStillRunsTheSubcommand(t *testing.T) {
	t.Setenv("RAY_HOME", t.TempDir())

	var out bytes.Buffer
	root := newRootCmd()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"-v", "profile", "list"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	got := out.String()
	if strings.Contains(got, "ray version") {
		t.Errorf("output = %q, want the profile list, not the version", got)
	}
	if !strings.Contains(got, "go") {
		t.Errorf("output = %q, want it to contain the seeded profiles", got)
	}
}

// O Cobra aceita qualquer argumento numa folha que não declara Args, e o
// resultado é um exit 0 sem efeito: `ray doctor fix` (em vez de `--fix`) não
// consertava nada e passava por sucesso. A varredura é da árvore inteira de
// propósito: uma folha nova tem de declarar o que aceita, mesmo que seja nada.
func TestLeafCommandsDeclareTheirArgs(t *testing.T) {
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		if !c.HasSubCommands() && c.Runnable() && c.Args == nil {
			t.Errorf("%q does not declare Args; a stray argument would be silently accepted", c.CommandPath())
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(newRootCmd())
}

// A consequência pelo caminho real: o erro tem de nomear o argumento, que é o
// que o usuário errou.
func TestDoctorRefusesAStrayArgument(t *testing.T) {
	t.Setenv("RAY_HOME", t.TempDir())

	var out bytes.Buffer
	root := newRootCmd()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"doctor", "fix"})

	err := root.Execute()
	if err == nil {
		t.Fatal("Execute() = nil, want an error for `doctor fix`")
	}
	if !strings.Contains(err.Error(), "fix") {
		t.Errorf("error = %q, want it to name the stray argument", err)
	}
}
