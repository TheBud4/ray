package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
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

	if err := runFirstRun(newEnv(t, "go-backend"), &out); err != nil {
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

	if err := runFirstRun(newEnv(t, ""), &out); err != nil {
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

// Editado de novo — RF-07 seguiu além da mensagem: init ai passou a cair no
// perfil `base` sem --profile, então o comando original volta a rodar de
// verdade e a tela recomenda ele puro de novo, sem flag.
func TestFirstRunOutsideAProjectSuggestsCreatingOne(t *testing.T) {
	var out bytes.Buffer

	if err := runFirstRun(t.TempDir(), &out); err != nil {
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
	// init ai não exige mais --profile (RF-07): a sugestão não deve mais
	// carregar a flag nem o desvio por `ray profile list`.
	for _, unwanted := range []string{"ray init ai --profile", "ray profile list"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("output = %q, want no %q — init ai runs without a profile now", got, unwanted)
		}
	}
}

// O CA que o I8 pede literalmente: `ray` sem args não pode mais cair no help
// cru do Cobra.
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

// A tela de abertura é a mais vista do CLI e não tem dependência obrigatória
// para avisar, então não pode pagar por checagem alguma: nenhum processo é
// criado. O PATH aponta só para executáveis que registram quem os chamou —
// qualquer `--version` disparado pela tela deixa rastro no log.
func TestRootWithoutArgsSpawnsNoProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("executáveis falsos são scripts sh")
	}
	bin := t.TempDir()
	log := filepath.Join(t.TempDir(), "spawned")
	script := "#!/bin/sh\necho \"$0 $*\" >> " + log + "\n"
	for _, name := range []string{"npx", "node", "jq", "python3", "python", "uv", "headroom", "graphify", "git"} {
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)

	var out bytes.Buffer
	root := newRootCmd()
	root.SetOut(&out)
	root.SetErr(&out)
	root.SetArgs([]string{})
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}

	if got, _ := os.ReadFile(log); len(got) > 0 {
		t.Errorf("a tela de abertura criou processos:\n%s", got)
	}
}
