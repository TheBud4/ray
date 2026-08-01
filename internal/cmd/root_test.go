package cmd

import (
	"bytes"
	"strings"
	"testing"
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
