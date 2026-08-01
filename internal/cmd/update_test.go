package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
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
	resetUpdateFlags(t)

	base := t.TempDir()
	profilesDir := filepath.Join(base, "profiles")
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	prof := &profile.Profile{
		Name:       "test",
		Components: []profile.Component{{Via: profile.ViaSkills, Skill: "s", Source: "o/r"}},
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

	home := update.Home{ProfilesDir: profilesDir, StoreDir: filepath.Join(base, "store")}
	opts := update.Options{Target: target, Out: &bytes.Buffer{}}

	// The skills acquisition fails (no seeding runner behind cleanCheckRunner
	// for npx) — exercises the "one or more steps failed" summary path.
	out := &bytes.Buffer{}
	err = runUpdate(cleanCheckRunner{}, cleanCheckRunner{}, opts, home, out)
	if err == nil {
		t.Fatal("runUpdate() = nil error, want error: component content never materializes under a plain stub runner")
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
