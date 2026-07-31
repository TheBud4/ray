package preflight

import (
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/runner"
)

func TestAdvicePointsAtTheFixWhenRayKnowsHowToInstall(t *testing.T) {
	c := Check{Name: "uv", Fix: []runner.Command{{Name: "sh"}}}

	if got := Advice(c, FromGate); got != "ray doctor --fix" {
		t.Errorf("Advice = %q, want %q", got, "ray doctor --fix")
	}
	if got := Advice(c, FromDoctor); got != "ray doctor --fix" {
		t.Errorf("Advice from doctor = %q, want %q", got, "ray doctor --fix")
	}
}

// Depois de o --fix rodar, apontar para ele de novo manda repetir o comando
// que acabou de não funcionar.
func TestAdviceStopsPointingAtTheFixAfterItRan(t *testing.T) {
	c := Check{Name: "uv", Fix: []runner.Command{{Name: "sh"}}}

	got := Advice(c, FromDoctorFix)
	if got != "automatic fix ran; install it manually" {
		t.Errorf("Advice = %q, want the manual-install text", got)
	}
	if strings.Contains(got, "--fix") {
		t.Errorf("Advice = %q, must not send the user back to --fix", got)
	}
}

func TestAdviceFallsBackToTheHintWithoutAFix(t *testing.T) {
	c := Check{Name: "npx", Hint: "install Node.js"}

	for _, from := range []Origin{FromGate, FromDoctor, FromDoctorFix} {
		if got := Advice(c, from); got != "install Node.js" {
			t.Errorf("Advice(from=%d) = %q, want the hint", from, got)
		}
	}
}

func TestAdviceIsEmptyWithNothingToSay(t *testing.T) {
	if got := Advice(Check{Name: "node"}, FromGate); got != "" {
		t.Errorf("Advice = %q, want empty when there is no fix and no hint", got)
	}
}

func TestMissingRequiredErrorNamesEachDependencyAndItsAdvice(t *testing.T) {
	err := &MissingRequiredError{
		Missing: []Check{
			{Name: "npx", Hint: "install Node.js"},
			{Name: "uv", Fix: []runner.Command{{Name: "sh"}}},
		},
		From: FromGate,
	}

	got := err.Error()
	for _, want := range []string{
		"missing required dependencies", "npx", "install Node.js",
		"uv", "ray doctor --fix",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Error() = %q, want it to contain %q", got, want)
		}
	}
}

// O rodapé só existe fora do doctor: lá dentro a tabela está impressa logo
// acima, e mandar rodar o comando em que já se está é ruído.
func TestMissingRequiredErrorFooterOnlyOutsideTheDoctor(t *testing.T) {
	missing := []Check{{Name: "npx", Hint: "install Node.js"}}
	const footer = "run `ray doctor` for the full table"

	gate := (&MissingRequiredError{Missing: missing, From: FromGate}).Error()
	if !strings.Contains(gate, footer) {
		t.Errorf("Error(FromGate) = %q, want the footer %q", gate, footer)
	}
	for _, from := range []Origin{FromDoctor, FromDoctorFix} {
		got := (&MissingRequiredError{Missing: missing, From: from}).Error()
		if strings.Contains(got, footer) {
			t.Errorf("Error(from=%d) = %q, want no footer inside the doctor", from, got)
		}
	}
}
