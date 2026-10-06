package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/update"
)

func resetUpdateFlags(t *testing.T) {
	t.Helper()
	prevProfile, prevForce, prevDryRun := flagUpdateProfile, flagUpdateForce, flagDryRun
	prevNoGlobal := flagNoGlobal
	t.Cleanup(func() {
		flagUpdateProfile, flagUpdateForce, flagDryRun = prevProfile, prevForce, prevDryRun
		flagNoGlobal = prevNoGlobal
	})
}

// cleanCheckRunner reports a clean git tree for `git status --porcelain`,
// so smoke tests can exercise runUpdate without a real git repo present.
type cleanCheckRunner struct{}

func (cleanCheckRunner) Run(_ context.Context, c runner.Command) (runner.Result, error) {
	if c.Name == "git" {
		return runner.Result{ExitCode: 0}, nil
	}
	return runner.Result{ExitCode: 0}, nil
}

func TestRunUpdatePrintsSummaryAndErrorsOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		// A falha é forçada com um diretório 0o555; o Windows ignora o bit de
		// escrita de diretório, então a cópia nunca falha ali.
		t.Skip("Windows has no write-permission bit on directories")
	}
	resetUpdateFlags(t)

	base := t.TempDir()
	profilesDir := filepath.Join(base, "profiles")
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	prof := &profile.Profile{
		Name:       "test",
		Components: []profile.Component{{Name: "s", Dest: ".claude/skills"}},
	}
	data, err := yaml.Marshal(prof)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, "test.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	target := t.TempDir()
	if err := os.MkdirAll(filepath.Join(target, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, ".claude", ".ray-profile"), []byte("test\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	componentsDir := filepath.Join(base, "components")
	if err := os.MkdirAll(filepath.Join(componentsDir, "s"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(componentsDir, "s", "SKILL.md"), []byte("# s"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A cópia é forçada a falhar sem rede: .claude/skills existe mas sem
	// permissão de escrita, então store.CopyTree não consegue criar
	// .claude/skills/s dentro dele.
	skillsDir := filepath.Join(target, ".claude", "skills")
	if err := os.MkdirAll(skillsDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(skillsDir, 0o755) })

	home := update.Home{ProfilesDir: profilesDir, StoreDir: filepath.Join(base, "store"), ComponentsDir: componentsDir}
	opts := update.Options{Target: target, Out: &bytes.Buffer{}}

	out := &bytes.Buffer{}
	err = runUpdate(cleanCheckRunner{}, cleanCheckRunner{}, opts, home, out)
	if err == nil {
		t.Fatal("runUpdate() = nil error, want error: the copy cannot land on a path blocked by a plain file")
	}
	if !strings.Contains(out.String(), "Failed") {
		t.Errorf("output = %q, want it to include a Failed section", out.String())
	}
}

// Quatro listas vazias significam que o perfil não tem componente — todo
// componente processado cai em alguma delas. Sem esta linha o comando termina
// com sucesso e sem uma palavra, e quem rodou não sabe se funcionou.
func TestPrintUpdateSummarySaysWhenThereWasNothing(t *testing.T) {
	var out bytes.Buffer
	printUpdateSummary(&out, update.Summary{})

	if got := strings.TrimSpace(out.String()); got != "no components to update" {
		t.Errorf("output = %q, want %q", got, "no components to update")
	}
}

// A recíproca, e é ela que impede a correção de virar ruído: execução que
// processou componente não ganha a linha.
func TestPrintUpdateSummaryStaysQuietWhenSomethingHappened(t *testing.T) {
	cases := []struct {
		name string
		sum  update.Summary
	}{
		{"updated", update.Summary{Updated: []string{"skills:o/r#s"}}},
		{"unchanged", update.Summary{Unchanged: []string{"skills:o/r#s"}}},
		{"skipped", update.Summary{Skipped: []string{"skills:o/r#s"}}},
		{"failed", update.Summary{Failed: []string{"skills:o/r#s"}}},
		{"warnings", update.Summary{Warnings: []string{"skills:o/r#s: fork"}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			printUpdateSummary(&out, tc.sum)

			if strings.Contains(out.String(), "no components to update") {
				t.Errorf("output = %q, want no empty-summary line when something happened", out.String())
			}
		})
	}
}

func TestUpdateCmdFlags(t *testing.T) {
	resetUpdateFlags(t)
	c := newUpdateCmd()

	if c.Use != "update [path]" {
		t.Errorf("Use = %q, want %q", c.Use, "update [path]")
	}
	if c.Flags().Lookup("profile") == nil {
		t.Error("missing --profile flag")
	}
	if c.Flags().Lookup("force") == nil {
		t.Error("missing --force flag")
	}
	if c.Flags().Lookup("no-global") == nil {
		t.Error("missing --no-global flag")
	}
}

func TestBuildUpdateOptionsMapsFlags(t *testing.T) {
	resetUpdateFlags(t)

	flagUpdateProfile, flagUpdateForce, flagNoGlobal, flagDryRun = "p", true, true, true
	opts := buildUpdateOptions("t", &bytes.Buffer{})

	if opts.Profile != "p" || opts.Target != "t" {
		t.Errorf("Profile/Target = %q/%q, want %q/%q", opts.Profile, opts.Target, "p", "t")
	}
	if !opts.Force || !opts.NoGlobal || !opts.DryRun {
		t.Errorf("Force/NoGlobal/DryRun = %v/%v/%v, want all true", opts.Force, opts.NoGlobal, opts.DryRun)
	}
}

// Só ferramenta atualizada: a seção aparece com nome próprio, e a linha de "sem
// componentes" segue valendo — nenhum componente foi processado.
func TestPrintUpdateSummaryListsToolsApartFromComponents(t *testing.T) {
	var out bytes.Buffer
	printUpdateSummary(&out, update.Summary{Tools: []string{"uv tool upgrade headroom-ai"}})

	got := out.String()
	if !strings.Contains(got, "Tools upgraded:\n  - uv tool upgrade headroom-ai") {
		t.Errorf("output = %q, want the tool under its own heading", got)
	}
	if strings.Contains(got, "Updated:") {
		t.Errorf("output = %q, want no Updated section for a tool", got)
	}
	if !strings.Contains(got, "no components to update") {
		t.Errorf("output = %q, want the no-components line when only tools ran", got)
	}
}
