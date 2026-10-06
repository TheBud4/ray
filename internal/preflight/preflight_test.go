package preflight

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/TheBud4/ray/internal/runner"
)

type stubLooker map[string]bool

func (s stubLooker) Look(name string) bool { return s[name] }

func TestRunSetsFoundFromLooker(t *testing.T) {
	l := stubLooker{"npx": true, "node": false, "python3.10+": true, "uv": true, "headroom": false, "graphify": false}
	checks := Run(l, true)

	got := map[string]bool{}
	for _, c := range checks {
		got[c.Name] = c.Found
	}
	for name, want := range l {
		if got[name] != want {
			t.Errorf("check %q Found = %v, want %v", name, got[name], want)
		}
	}
}

func TestRunRequiredWithNeedPython(t *testing.T) {
	l := stubLooker{} // nothing found
	checks := Run(l, true)

	required := map[string]bool{}
	for _, c := range checks {
		required[c.Name] = c.Required
	}
	want := map[string]bool{
		"npx": true, "node": false, "python3.10+": true, "uv": true,
		"headroom": false, "graphify": false,
	}
	for name, w := range want {
		if required[name] != w {
			t.Errorf("check %q Required = %v, want %v (needPython=true)", name, required[name], w)
		}
	}
}

func TestRunRequiredWithoutNeedPython(t *testing.T) {
	l := stubLooker{}
	checks := Run(l, false)

	required := map[string]bool{}
	for _, c := range checks {
		required[c.Name] = c.Required
	}
	if required["python3.10+"] {
		t.Error("python3.10+ Required = true, want false when needPython=false")
	}
	if required["uv"] {
		t.Error("uv Required = true, want false when needPython=false")
	}
	if !required["npx"] {
		t.Error("npx Required = false, want true regardless of needPython")
	}
}

func TestRunFixCommandsPresentWhereExpected(t *testing.T) {
	checks := Run(stubLooker{}, true)

	wantFix := map[string]bool{
		"npx": false, "node": false, "jq": false, "python3.10+": false,
		"uv": true, "headroom": true, "graphify": true,
	}
	for _, c := range checks {
		want, ok := wantFix[c.Name]
		if !ok {
			t.Fatalf("unexpected check %q", c.Name)
		}
		hasFix := len(c.Fix) > 0
		if hasFix != want {
			t.Errorf("check %q has Fix = %v, want %v", c.Name, hasFix, want)
		}
	}
}

func TestMissingRequiredFiltersByRequiredAndFound(t *testing.T) {
	l := stubLooker{"npx": true, "python3.10+": false, "uv": true, "headroom": false, "graphify": false}
	checks := Run(l, true)

	missing := MissingRequired(checks)
	names := map[string]bool{}
	for _, c := range missing {
		names[c.Name] = true
	}
	if !names["python3.10+"] {
		t.Error("MissingRequired should include python3.10+ (required, missing)")
	}
	if names["headroom"] || names["graphify"] {
		t.Error("MissingRequired should not include non-required checks even if missing")
	}
	if names["npx"] {
		t.Error("MissingRequired should not include npx (present)")
	}
}

func TestRunIncludesOptionalJQ(t *testing.T) {
	l := stubLooker{"npx": true, "jq": false}
	checks := Run(l, false)

	var jq *Check
	for i := range checks {
		if checks[i].Name == "jq" {
			jq = &checks[i]
		}
	}
	if jq == nil {
		t.Fatal("Run() has no jq check; the scaffolded hooks depend on it")
	}
	if jq.Required {
		t.Error("jq Required = true, want false: a hook without jq no-ops, it does not break")
	}
	if jq.Hint == "" {
		t.Error("jq Hint is empty; without it ray doctor cannot say what to do")
	}
}

// uvFixCommand devolve o Fix da checagem do uv, como o doctor o executa.
func uvFixCommand(t *testing.T) runner.Command {
	t.Helper()
	for _, c := range Run(stubMissing{}, true) {
		if c.Name == "uv" && len(c.Fix) == 1 {
			return c.Fix[0]
		}
	}
	t.Fatal("no fix command for uv")
	return runner.Command{}
}

type stubMissing struct{}

func (stubMissing) Look(string) bool { return false }

func fakeCurl(t *testing.T, body string) (pathEnv string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "curl"), []byte("#!/bin/sh\n"+body+"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir + string(os.PathListSeparator) + os.Getenv("PATH")
}

// O script roda de verdade, com um curl falso na frente do PATH: o exit do curl
// tem de ser o exit do fix. Sem rede, `curl | sh` devolvia o exit do sh (0) e o
// doctor anunciava um uv que nunca foi instalado.
func TestUVInstallScriptPropagatesACurlFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix shell script")
	}
	cmd := uvFixCommand(t)
	cmd.Env = map[string]string{"PATH": fakeCurl(t, "echo 'curl: (6) Could not resolve host' >&2; exit 6")}

	res, err := runner.ExecRunner{}.Run(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode == 0 {
		t.Errorf("exit = 0 with a failing curl; want non-zero (stderr = %q)", res.Stderr)
	}
}

func TestUVInstallScriptRunsWhatCurlDownloads(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("posix shell script")
	}
	marker := filepath.Join(t.TempDir(), "installed")
	cmd := uvFixCommand(t)
	cmd.Env = map[string]string{"PATH": fakeCurl(t, "echo 'touch "+marker+"'")}

	res, err := runner.ExecRunner{}.Run(context.Background(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit = %d, stderr = %q; want 0", res.ExitCode, res.Stderr)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("the downloaded script did not run (%v)", err)
	}
}
