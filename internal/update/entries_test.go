package update

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/report"
	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/store"
)

// installOnDisk põe o componente s no projeto com o conteúdo dado e grava a
// linha-base com o hash de baseline (vazio = sem linha-base).
func installOnDisk(t *testing.T, target, content, baseline string) {
	t.Helper()
	dir := filepath.Join(target, ".claude", "skills", "s")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if baseline == "" {
		return
	}
	h, err := store.HashTree(seedTempFile(t, baseline))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ProjectBaseline(target).SetPristine(coordS, h); err != nil {
		t.Fatal(err)
	}
}

func updateWith(t *testing.T, home Home, target string, fr runner.Runner, opts Options) Summary {
	t.Helper()
	opts.Target = target
	opts.Out = &bytes.Buffer{}
	sum, err := Run(fr, cleanGitCheck(), opts, home)
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	return sum
}

func assertHasEntries(t *testing.T, sum Summary, want ...report.Entry) {
	t.Helper()
	for _, w := range want {
		if !slices.Contains(sum.Entries, w) {
			t.Errorf("Entries lacks %+v\nEntries = %+v", w, sum.Entries)
		}
	}
}

// names devolve os nomes das entradas que passam por keep.
func names(entries []report.Entry, keep func(report.Entry) bool) []string {
	var out []string
	for _, e := range entries {
		if keep(e) {
			out = append(out, e.Name)
		}
	}
	return out
}

// entryNames devolve os nomes das entradas de um tipo e resultado, na ordem em
// que foram gravadas.
func entryNames(entries []report.Entry, kind report.Kind, o report.Outcome) []string {
	return names(entries, func(e report.Entry) bool { return e.Kind == kind && e.Outcome == o })
}

// withOutcome devolve os nomes das entradas com o resultado o, de qualquer
// tipo, na ordem em que foram gravadas.
func withOutcome(entries []report.Entry, o report.Outcome) []string {
	return names(entries, func(e report.Entry) bool { return e.Outcome == o })
}

// entryFor devolve a entrada de um tipo e nome.
func entryFor(entries []report.Entry, kind report.Kind, name string) (report.Entry, bool) {
	for _, e := range entries {
		if e.Kind == kind && e.Name == name {
			return e, true
		}
	}
	return report.Entry{}, false
}

func TestRunEntriesRecordAnUpdatedComponentAndTheUpgradedTools(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "# new upstream")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	writeProfileRecord(t, target, "test")
	installOnDisk(t, target, "# old", "# old") // intocado: acompanha a fonte

	sum := updateWith(t, home, target, &runner.FakeRunner{}, Options{})

	if sum.Profile != "test" {
		t.Errorf("Profile = %q, want the resolved recipe name", sum.Profile)
	}
	assertHasEntries(t, sum,
		report.Entry{Kind: report.Component, Name: "s", Outcome: report.Updated},
		report.Entry{Kind: report.Command, Name: "uv tool upgrade headroom-ai", Outcome: report.Updated, Detail: "upgraded"},
		report.Entry{Kind: report.Command, Name: "uv tool upgrade graphifyy", Outcome: report.Updated, Detail: "upgraded"},
	)
}

func TestRunEntriesRecordAnUnchangedComponent(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "# same")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	writeProfileRecord(t, target, "test")
	installOnDisk(t, target, "# same", "# same")

	sum := updateWith(t, home, target, &runner.FakeRunner{}, Options{NoGlobal: true})

	assertHasEntries(t, sum, report.Entry{Kind: report.Component, Name: "s", Outcome: report.Unchanged})
}

func TestRunEntriesRecordAnEditedComponentAsKeptWithTheReason(t *testing.T) {
	for name, dry := range map[string]bool{"real run": false, "dry-run": true} {
		t.Run(name, func(t *testing.T) {
			home := newHome(t)
			seedComponent(t, home, "# new upstream")
			writeProfile(t, home.ProfilesDir, testProfile())
			target := t.TempDir()
			writeProfileRecord(t, target, "test")
			installOnDisk(t, target, "# my local edit", "# original")

			sum := updateWith(t, home, target, &runner.FakeRunner{}, Options{NoGlobal: true, DryRun: dry})

			var got *report.Entry
			for i, e := range sum.Entries {
				if e.Kind == report.Component && e.Name == "s" {
					got = &sum.Entries[i]
				}
			}
			if got == nil || got.Outcome != report.Kept || !strings.HasPrefix(got.Detail, "edited locally") {
				t.Errorf("component entry = %+v, want Kept with the reason starting \"edited locally\"", got)
			}
		})
	}
}

func TestRunEntriesRecordAMissingComponentAsKeptWithItsPath(t *testing.T) {
	home := newHome(t)
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	writeProfileRecord(t, target, "test")

	sum := updateWith(t, home, target, &runner.FakeRunner{}, Options{NoGlobal: true})

	assertHasEntries(t, sum, report.Entry{
		Kind: report.Component, Name: "s", Outcome: report.Kept,
		Detail: "not found at " + filepath.Join(home.ComponentsDir, "s"),
	})
}

func TestRunEntriesRecordAFailedToolUpgradeWithItsReason(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "# s")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	writeProfileRecord(t, target, "test")
	fr := &runner.FakeRunner{Results: map[string]runner.Result{
		"uv tool upgrade headroom-ai": {ExitCode: 1, Stderr: "error: network unreachable\n"},
	}}

	sum := updateWith(t, home, target, fr, Options{})

	assertHasEntries(t, sum, report.Entry{
		Kind: report.Command, Name: "uv tool upgrade headroom-ai", Outcome: report.Failed,
		Detail: "exit 1: error: network unreachable",
	})
	if !sum.HadFailure {
		t.Error("HadFailure = false, want the failed upgrade to fail the run")
	}
}
