package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newEnv monta um .claude/ mínimo em t.TempDir() e devolve o caminho.
func newEnv(t *testing.T, profileName string) string {
	t.Helper()
	target := t.TempDir()
	skill := filepath.Join(target, ".claude", "skills", "tdd", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skill), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(skill, []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if profileName != "" {
		rec := filepath.Join(target, ".claude", ".ray-profile")
		if err := os.WriteFile(rec, []byte(profileName+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return target
}

// Dentro de um projeto a tela reconhece o ambiente e faz ponte para o status,
// em vez de mandar criar um projeto que já existe.
func TestFirstRunInsideAProjectPointsAtTheSession(t *testing.T) {
	var out bytes.Buffer

	if err := runFirstRun(stubLooker{"npx": true}, newEnv(t, "go-backend"), &out); err != nil {
		t.Fatalf("runFirstRun() error = %v", err)
	}
	got := out.String()
	for _, want := range []string{"profile: go-backend", "1 skill", "claude", "ray status"} {
		if !strings.Contains(got, want) {
			t.Errorf("output = %q, want it to contain %q", got, want)
		}
	}
	if strings.Contains(got, "ray new go my-app") {
		t.Errorf("output = %q, want no suggestion to create a project inside one", got)
	}
}

// Ambiente copiado à mão: mostra o inventário e omite o perfil, em vez de
// inventar um.
func TestFirstRunInsideAProjectWithoutARecordedProfile(t *testing.T) {
	var out bytes.Buffer

	if err := runFirstRun(stubLooker{"npx": true}, newEnv(t, ""), &out); err != nil {
		t.Fatalf("runFirstRun() error = %v", err)
	}
	got := out.String()
	if strings.Contains(got, "profile:") {
		t.Errorf("output = %q, want no profile line without a .ray-profile", got)
	}
	if !strings.Contains(got, "1 skill") {
		t.Errorf("output = %q, want the inventory anyway", got)
	}
}

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
