package cmd

import (
	"bytes"
	"strings"
	"testing"
)

func TestFirstRunOutsideAProjectSuggestsCreatingOne(t *testing.T) {
	var out bytes.Buffer

	if err := runFirstRun(stubLooker{"npx": true}, t.TempDir(), &out); err != nil {
		t.Fatalf("runFirstRun() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{
		"Next steps:", "ray new go my-app", "ray init ai", "`ray --help`",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output = %q, want it to contain %q", got, want)
		}
	}
}

// Sem required faltando não há linha de dependência nenhuma. O ⚠ só significa
// alguma coisa porque o silêncio é o normal — é a regra que o ray status já
// segue, e uma linha "ok" fixa na tela mais vista do CLI gastaria a mesma
// atenção que o alerta precisa.
func TestFirstRunSaysNothingWhenDepsAreFine(t *testing.T) {
	var out bytes.Buffer

	if err := runFirstRun(stubLooker{"npx": true}, t.TempDir(), &out); err != nil {
		t.Fatalf("runFirstRun() error = %v", err)
	}
	if strings.Contains(out.String(), "⚠") {
		t.Errorf("output = %q, want no warning marker when nothing is missing", out.String())
	}
}

// Required faltando é alerta na tela, não exit ≠ 0: quem erra por dependência
// é o doctor, porque ali o próximo comando quebra de verdade.
func TestFirstRunWarnsAboutMissingRequiredWithoutFailing(t *testing.T) {
	var out bytes.Buffer

	err := runFirstRun(stubLooker{}, t.TempDir(), &out) // npx ausente
	if err != nil {
		t.Fatalf("runFirstRun() error = %v, want nil — a missing dep is a warning here", err)
	}
	got := out.String()
	for _, want := range []string{"⚠", "npx", "ray doctor"} {
		if !strings.Contains(got, want) {
			t.Errorf("output = %q, want it to contain %q", got, want)
		}
	}
}

// O CA que o I8 pede literalmente: `ray` sem args não pode mais cair no help
// cru do Cobra. Roda o preflight real (só o npx), e a asserção vale com ou sem
// ele instalado.
func TestRootWithoutArgsPrintsTheScreenNotTheRawHelp(t *testing.T) {
	var out bytes.Buffer
	root := newRootCmd()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Next steps:") {
		t.Errorf("output = %q, want the first-run screen", got)
	}
	if strings.Contains(got, "Available Commands") {
		t.Errorf("output = %q, want the screen instead of Cobra's raw help", got)
	}
}

// Guarda contra o RunE sequestrar o help: `ray --help` continua listando tudo.
func TestRootHelpStillListsEveryCommand(t *testing.T) {
	var out bytes.Buffer
	root := newRootCmd()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{"--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"Available Commands", "status", "doctor", "init"} {
		if !strings.Contains(got, want) {
			t.Errorf("help = %q, want it to contain %q", got, want)
		}
	}
}
