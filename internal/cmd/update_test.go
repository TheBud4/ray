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
	"github.com/TheBud4/ray/internal/report"
	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/ui"
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
	if !strings.Contains(out.String(), "  ✗ s  ") || !strings.Contains(out.String(), "1 failed") {
		t.Errorf("output = %q, want the failed component line and the failed count", out.String())
	}
}

// Sem nenhuma entrada de componente e sem aviso, o perfil não tem componente —
// todo componente processado vira uma entrada. Sem esta linha o comando termina
// com sucesso e sem uma palavra, e quem rodou não sabe se funcionou.
func TestPrintUpdateSummarySaysWhenThereWasNothing(t *testing.T) {
	var out bytes.Buffer
	printUpdateSummary(&out, update.Summary{Profile: "demo"})

	if want, got := "update · demo\n\nno components to update\n", out.String(); got != want {
		t.Errorf("output = %q, want %q", got, want)
	}
}

// A recíproca, e é ela que impede a correção de virar ruído: execução que
// processou componente (ou avisou de algo) não ganha a linha.
func TestPrintUpdateSummaryStaysQuietWhenSomethingHappened(t *testing.T) {
	entry := func(o report.Outcome) []report.Entry {
		return []report.Entry{{Kind: report.Component, Name: "skills:o/r#s", Outcome: o}}
	}
	cases := []struct {
		name string
		sum  update.Summary
	}{
		{"updated", update.Summary{Entries: entry(report.Updated)}},
		{"unchanged", update.Summary{Entries: entry(report.Unchanged)}},
		{"kept", update.Summary{Entries: entry(report.Kept)}},
		{"failed", update.Summary{Entries: entry(report.Failed)}},
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

// Só ferramenta atualizada: a seção Tools aparece, a ferramenta conta em
// "updated", e a linha de "sem componentes" segue valendo — nenhum componente
// foi processado.
func TestPrintUpdateSummaryListsToolsApartFromComponents(t *testing.T) {
	var out bytes.Buffer
	printUpdateSummary(&out, update.Summary{
		Profile: "demo",
		Entries: []report.Entry{{Kind: report.Command, Name: "uv tool upgrade headroom-ai", Outcome: report.Updated, Detail: "upgraded"}},
	})

	want := "update · demo\n" +
		"\n" +
		"Tools\n" +
		"  ~ uv tool upgrade headroom-ai  upgraded\n" +
		"\n" +
		"no components to update\n" +
		"\n" +
		"1 updated\n"
	if got := out.String(); got != want {
		t.Errorf("output =\n%q\nwant\n%q", got, want)
	}
	assertNoEscape(t, out.String())
}

// A tela aprovada do update (com --no-global): componentes atualizado,
// inalterado e preservado, a dica do --force uma vez, e o resumo.
func TestPrintUpdateSummaryShowsComponentsAndTheForceHint(t *testing.T) {
	var out bytes.Buffer
	printUpdateSummary(&out, update.Summary{
		Profile: "demo",
		Entries: []report.Entry{
			{Kind: report.Component, Name: "t", Outcome: report.Updated},
			{Kind: report.Component, Name: "s", Outcome: report.Unchanged},
			{Kind: report.Component, Name: "u", Outcome: report.Kept, Detail: "edited locally (differs from last pristine); use --force to overwrite"},
		},
	})
	want := "update · demo\n" +
		"\n" +
		"Components\n" +
		"  ~ t  updated\n" +
		"  = s  unchanged\n" +
		"  = u  kept — edited locally\n" +
		"\n" +
		"⚠ use --force to overwrite the components kept above\n" +
		"\n" +
		"1 updated · 1 unchanged · 1 kept\n"
	if got := out.String(); got != want {
		t.Errorf("output =\n%q\nwant\n%q", got, want)
	}
	assertNoEscape(t, out.String())
}

// Componente ausente é uma entrada Kept cujo detalhe é o caminho: sai sem o
// prefixo "kept —", com a home abreviada, e não pede --force (não há edição).
func TestPrintUpdateSummaryShowsAMissingComponentWithItsPath(t *testing.T) {
	home := t.TempDir()
	fixHome(t, home)
	missing := filepath.Join(home, "components", "x")
	var out bytes.Buffer
	printUpdateSummary(&out, update.Summary{
		Profile: "demo",
		Entries: []report.Entry{{Kind: report.Component, Name: "x", Outcome: report.Kept, Detail: "not found at " + missing}},
	})
	want := "  = x  not found at ~" + string(filepath.Separator) + filepath.Join("components", "x") + "\n"
	if !strings.Contains(out.String(), want) {
		t.Errorf("output = %q, want the line %q", out.String(), want)
	}
	if strings.Contains(out.String(), "kept —") || strings.Contains(out.String(), "--force") {
		t.Errorf("output = %q, a missing component is neither \"kept —\" nor fixed by --force", out.String())
	}
}

// Falha de ferramenta e de componente aparecem como ✗ com o motivo; o resumo
// conta as duas.
func TestPrintUpdateSummaryShowsFailures(t *testing.T) {
	var out bytes.Buffer
	printUpdateSummary(&out, update.Summary{
		Profile: "demo",
		Entries: []report.Entry{
			{Kind: report.Command, Name: "uv tool upgrade headroom-ai", Outcome: report.Failed, Detail: "exit 2"},
			{Kind: report.Component, Name: "s", Outcome: report.Failed, Detail: "permission denied"},
		},
		HadFailure: true,
	})
	want := "update · demo\n" +
		"\n" +
		"Tools\n" +
		"  ✗ uv tool upgrade headroom-ai  exit 2\n" +
		"\n" +
		"Components\n" +
		"  ✗ s  permission denied\n" +
		"\n" +
		"2 failed\n"
	if got := out.String(); got != want {
		t.Errorf("output =\n%q\nwant\n%q", got, want)
	}
}

// Os avisos que sobram saem como ⚠ antes do resumo, e a dica do --force vem
// depois deles.
func TestPrintUpdateSummaryShowsLeftoverWarningsBeforeTheForceHint(t *testing.T) {
	var out bytes.Buffer
	printUpdateSummary(&out, update.Summary{
		Profile:  "demo",
		Entries:  []report.Entry{{Kind: report.Component, Name: "u", Outcome: report.Kept, Detail: "edited locally (x)"}},
		Warnings: []string{"baseline not recorded"},
	})
	want := "\n⚠ baseline not recorded\n⚠ use --force to overwrite the components kept above\n\n1 kept\n"
	if !strings.HasSuffix(out.String(), want) {
		t.Errorf("output = %q, want it to end with %q", out.String(), want)
	}
}

// A cor reforça o marcador, nunca o substitui: tirados os escapes, a saída
// colorida é a mesma do texto simples; e cada papel tem a sua cor.
func TestPrintUpdateSummaryColorsEachRole(t *testing.T) {
	sum := update.Summary{
		Profile: "demo",
		Entries: []report.Entry{
			{Kind: report.Component, Name: "t", Outcome: report.Updated},
			{Kind: report.Component, Name: "s", Outcome: report.Unchanged},
			{Kind: report.Component, Name: "u", Outcome: report.Kept, Detail: "edited locally (x)"},
			{Kind: report.Component, Name: "f", Outcome: report.Failed, Detail: "boom"},
		},
		HadFailure: true,
	}
	var plain, colored bytes.Buffer
	printUpdateSummary(&plain, sum)
	printUpdateSummary(ui.New(&colored, true), sum)

	if got := stripEscapes(colored.String()); got != plain.String() {
		t.Errorf("colored output without escapes =\n%q\nwant the plain output\n%q", got, plain.String())
	}
	for _, want := range []string{
		esc + "[33m~" + esc + "[0m t  " + esc + "[90mupdated" + esc + "[0m\n",
		esc + "[90m=" + esc + "[0m " + esc + "[90ms" + esc + "[0m  " + esc + "[90munchanged" + esc + "[0m\n",
		esc + "[90m=" + esc + "[0m " + esc + "[90mu" + esc + "[0m  " + esc + "[90mkept — edited locally" + esc + "[0m\n",
		esc + "[33m⚠" + esc + "[0m use --force to overwrite the components kept above\n",
		esc + "[31m1 failed" + esc + "[0m",
		esc + "[33m1 updated" + esc + "[0m",
		esc + "[90m1 unchanged" + esc + "[0m",
		esc + "[90m1 kept" + esc + "[0m",
	} {
		if !strings.Contains(colored.String(), want) {
			t.Errorf("colored output = %q, want it to contain %q", colored.String(), want)
		}
	}
}
