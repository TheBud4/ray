package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/runner"
)

type stubLooker map[string]bool

func (s stubLooker) Look(name string) bool { return s[name] }

func TestRunDoctorAllPresentNoError(t *testing.T) {
	l := stubLooker{"npx": true, "node": true, "python3.10+": true, "uv": true, "headroom": true, "graphify": true}
	var out bytes.Buffer

	err := runDoctor(l, &runner.FakeRunner{}, false, &out)
	if err != nil {
		t.Fatalf("runDoctor() error = %v, want nil", err)
	}
	if !strings.Contains(out.String(), "npx") {
		t.Errorf("output = %q, want it to contain the table with npx", out.String())
	}
}

func TestRunDoctorMissingRequiredWithoutFix(t *testing.T) {
	l := stubLooker{"node": true} // npx missing (required)
	var out bytes.Buffer

	err := runDoctor(l, &runner.FakeRunner{}, false, &out)
	if err == nil {
		t.Fatal("runDoctor() = nil error, want error when a required dep is missing")
	}
	if !strings.Contains(err.Error(), "ray doctor --fix") {
		t.Errorf("error = %q, want it to hint at `ray doctor --fix`", err.Error())
	}
}

// A tabela tinha NAME/FOUND/REQUIRED e escondia o Hint, que é a única coluna
// que diz o que fazer. Só para o que falta: aconselhar sobre algo presente é
// ruído.
func TestRunDoctorTableShowsAdviceOnlyForWhatIsMissing(t *testing.T) {
	l := stubLooker{"node": true, "python3.10+": true, "uv": true, "headroom": true, "graphify": true}
	var out bytes.Buffer

	_ = runDoctor(l, &runner.FakeRunner{}, false, &out)

	got := out.String()
	if !strings.Contains(got, "HINT") {
		t.Errorf("table = %q, want a HINT column", got)
	}
	if !strings.Contains(got, "install Node.js") {
		t.Errorf("table = %q, want the advice for the missing npx", got)
	}
	for _, line := range strings.Split(got, "\n") {
		if strings.HasPrefix(line, "node ") && strings.Contains(line, "install") {
			t.Errorf("line = %q, want no advice for a dependency that was found", line)
		}
	}
}

// Sem nada a aconselhar, a coluna não existe — e não sobra espaço em branco no
// fim de cada linha. Mesmo princípio que tirou a linha "deps: ok" da tela de
// abertura: coluna de nada quando está tudo bem é ruído.
func TestRunDoctorTableOmitsTheHintColumnWhenThereIsNothingToSay(t *testing.T) {
	l := stubLooker{"npx": true, "node": true, "jq": true, "python3.10+": true,
		"uv": true, "headroom": true, "graphify": true}
	var out bytes.Buffer

	if err := runDoctor(l, &runner.FakeRunner{}, false, &out); err != nil {
		t.Fatalf("runDoctor() error = %v", err)
	}
	got := out.String()
	if strings.Contains(got, "HINT") {
		t.Errorf("table = %q, want no HINT column when nothing is missing", got)
	}
	for _, line := range strings.Split(strings.TrimRight(got, "\n"), "\n") {
		if strings.HasSuffix(line, " ") {
			t.Errorf("line = %q, want no trailing whitespace", line)
		}
	}
}

// Depois de o --fix rodar e não resolver, mandar rodar `ray doctor --fix` de
// novo é mandar repetir o comando que acabou de falhar.
func TestRunDoctorAfterFixDoesNotPointBackAtFix(t *testing.T) {
	l := stubLooker{"npx": true, "node": true, "python3.10+": true} // uv segue ausente
	var out bytes.Buffer

	err := runDoctor(l, &runner.FakeRunner{}, true, &out)
	if err == nil {
		t.Fatal("runDoctor() = nil error, want error while uv is still missing")
	}
	if strings.Contains(err.Error(), "ray doctor --fix") {
		t.Errorf("error = %q, must not send the user back to the fix that just ran", err.Error())
	}
	if !strings.Contains(err.Error(), "install it manually") {
		t.Errorf("error = %q, want the manual-install advice", err.Error())
	}
}

// flippingLooker simulates a dependency becoming available after Fix runs:
// it answers from `before` until the fixRunner has recorded any call, then
// from `after` — proving runDoctor re-checks after fixing.
type flippingLooker struct {
	before, after map[string]bool
	fr            *runner.FakeRunner
}

func (f flippingLooker) Look(name string) bool {
	m := f.before
	if len(f.fr.Calls) > 0 {
		m = f.after
	}
	return m[name]
}

// Editado para RF-06: o teste original esperava que headroom/graphify
// rodassem seus Fix na mesma passada em que uv acabou de ser instalado — mas
// isso falha de verdade fora do fake, porque o PATH do processo não muda
// depois do script curl|sh. O contrato novo é pular os dois nesta passada;
// ver TestRunDoctorFixSkipsDependentsWhenUVWasJustInstalled.
func TestRunDoctorFixInstallsAndRechecks(t *testing.T) {
	fr := &runner.FakeRunner{}
	l := flippingLooker{
		before: map[string]bool{"npx": true, "node": true},
		after:  map[string]bool{"npx": true, "node": true, "uv": true, "python3.10+": true},
		fr:     fr,
	}
	var out bytes.Buffer

	err := runDoctor(l, fr, true, &out)
	if err != nil {
		t.Fatalf("runDoctor() error = %v, want nil after fix", err)
	}

	wantCalled := "sh -c curl -LsSf https://astral.sh/uv/install.sh | sh"
	found := false
	for _, c := range fr.Calls {
		if c.String() == wantCalled {
			found = true
		}
		if c.Name == "uv" {
			t.Errorf("Calls = %v, want no `uv tool install ...` call in the same pass uv was just installed", fr.Calls)
		}
	}
	if !found {
		t.Errorf("expected fix command %q to run, but it did not (Calls = %v)", wantCalled, fr.Calls)
	}
}

// Depois de instalar o uv, rodar `uv tool install ...` no mesmo processo
// falharia com "executable file not found" — o PATH do processo Go não muda
// por um script curl|sh ter rodado num subshell. runDoctor pula headroom e
// graphify nesta passada e explica por quê, em vez de deixá-los falhar com
// um erro de exec cru.
func TestRunDoctorFixSkipsDependentsWhenUVWasJustInstalled(t *testing.T) {
	l := stubLooker{"npx": true, "node": true} // uv, headroom, graphify ausentes
	fr := &runner.FakeRunner{}
	var out bytes.Buffer

	_ = runDoctor(l, fr, true, &out)

	for _, c := range fr.Calls {
		if c.Name == "uv" {
			t.Errorf("Calls = %v, want no `uv tool install ...` call in the same pass uv was just installed", fr.Calls)
		}
	}
	got := out.String()
	for _, name := range []string{"headroom", "graphify"} {
		if !strings.Contains(got, "skip fix "+name) {
			t.Errorf("output = %q, want a skip message naming %s", got, name)
		}
	}
	if !strings.Contains(got, "ray doctor --fix") {
		t.Errorf("output = %q, want it to tell the user to rerun `ray doctor --fix`", got)
	}
}

// uv já presente (não corrigido nesta rodada) não tem o problema de PATH: o
// Fix de headroom/graphify deve rodar normalmente.
func TestRunDoctorFixRunsDependentsWhenUVWasAlreadyPresent(t *testing.T) {
	l := stubLooker{"npx": true, "node": true, "python3.10+": true, "uv": true} // headroom, graphify ausentes
	fr := &runner.FakeRunner{}
	var out bytes.Buffer

	_ = runDoctor(l, fr, true, &out)

	wantCalled := map[string]bool{
		"uv tool install headroom-ai[mcp]": false,
		"uv tool install graphifyy":        false,
	}
	for _, c := range fr.Calls {
		if _, ok := wantCalled[c.String()]; ok {
			wantCalled[c.String()] = true
		}
	}
	for cmdStr, called := range wantCalled {
		if !called {
			t.Errorf("expected fix command %q to run when uv was already present, but it did not (Calls = %v)", cmdStr, fr.Calls)
		}
	}
}

func TestRunDoctorFixDryRunDoesNotExecute(t *testing.T) {
	dryRunner := runner.ExecRunner{DryRun: true, Out: &bytes.Buffer{}}
	l := stubLooker{"npx": true, "node": true}
	var out bytes.Buffer

	// uv/headroom/graphify all missing and have Fix; use the real ExecRunner
	// in DryRun mode as fixRunner to prove it never spawns a process.
	err := runDoctor(l, dryRunner, true, &out)
	if err == nil {
		t.Fatal("runDoctor() = nil error, want error: python3.10+/uv still missing after a dry-run fix")
	}
}
