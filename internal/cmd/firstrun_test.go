package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/ui"
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

const firstRunTitle = "ray — versioned AI environments\n"
const firstRunFooter = "\n`ray --help` lists every command\n"

// Dentro de um projeto a tela reconhece o ambiente e faz ponte para o status,
// em vez de mandar criar um projeto que já existe.
func TestFirstRunInsideAProjectPointsAtTheSession(t *testing.T) {
	var out bytes.Buffer

	if err := runFirstRun(newEnv(t, "go-backend"), &out); err != nil {
		t.Fatalf("runFirstRun() error = %v", err)
	}
	assertNoEscape(t, out.String())
	want := firstRunTitle + "\ngo-backend · 1 skill\n\nNext steps\n" +
		"  claude      start the session\n" +
		"  ray status  diagnose the environment\n" + firstRunFooter
	if out.String() != want {
		t.Errorf("output =\n%q\nwant\n%q", out.String(), want)
	}
}

// Ambiente copiado à mão: mostra o inventário e omite o perfil, em vez de
// inventar um — e sem título, para não imprimir um " · 1 skill" solto.
func TestFirstRunInsideAProjectWithoutARecordedProfile(t *testing.T) {
	var out bytes.Buffer

	if err := runFirstRun(newEnv(t, ""), &out); err != nil {
		t.Fatalf("runFirstRun() error = %v", err)
	}
	want := firstRunTitle + "\n1 skill\n\nNext steps\n" +
		"  claude      start the session\n" +
		"  ray status  diagnose the environment\n" + firstRunFooter
	if out.String() != want {
		t.Errorf("output =\n%q\nwant\n%q", out.String(), want)
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
	assertNoEscape(t, out.String())
	want := firstRunTitle + "\nNext steps\n" +
		"  ray new go my-app  new project, environment included\n" +
		"  ray init ai        environment only, in this directory\n" + firstRunFooter
	if out.String() != want {
		t.Errorf("output =\n%q\nwant\n%q", out.String(), want)
	}
	// init ai não exige mais --profile (RF-07): a sugestão não deve mais
	// carregar a flag nem o desvio por `ray profile list`.
	for _, unwanted := range []string{"ray init ai --profile", "ray profile list"} {
		if strings.Contains(out.String(), unwanted) {
			t.Errorf("output = %q, want no %q — init ai runs without a profile now", out.String(), unwanted)
		}
	}
}

func TestFirstRunColorsTheProfileTheSectionAndTheCommands(t *testing.T) {
	var out bytes.Buffer
	if err := runFirstRun(newEnv(t, "go-backend"), ui.New(&out, true)); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		esc + "[1mgo-backend" + esc + "[22m " + esc + "[90m· 1 skill" + esc + "[0m\n",
		esc + "[1mNext steps" + esc + "[22m\n",
		"  " + esc + "[36mclaude" + esc + "[0m      start the session\n",
		"  " + esc + "[36mray status" + esc + "[0m  diagnose the environment\n",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("colored first run lacks %q in\n%q", want, out.String())
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
	if !strings.Contains(got, "Next steps\n") || strings.Contains(got, "Next steps:") {
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
