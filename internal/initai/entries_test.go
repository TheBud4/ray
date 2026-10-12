package initai

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

// names devolve os nomes das entradas que passam por keep, na ordem em que
// foram gravadas.
func names(entries []report.Entry, keep func(report.Entry) bool) []string {
	var out []string
	for _, e := range entries {
		if keep(e) {
			out = append(out, e.Name)
		}
	}
	return out
}

// assertEntriesMirrorTheLists confere que os itens tipados dizem o mesmo que as
// listas planas, na mesma ordem: as listas continuam valendo até o cmd passar a
// ler só os itens.
func assertEntriesMirrorTheLists(t *testing.T, sum Summary) {
	t.Helper()
	outcome := func(o report.Outcome) func(report.Entry) bool {
		return func(e report.Entry) bool { return e.Outcome == o }
	}
	if got := names(sum.Entries, outcome(report.Installed)); !slices.Equal(got, sum.Installed) {
		t.Errorf("Installed entries = %v, Installed list = %v", got, sum.Installed)
	}
	if got := names(sum.Entries, outcome(report.Failed)); !slices.Equal(got, sum.Failed) {
		t.Errorf("Failed entries = %v, Failed list = %v", got, sum.Failed)
	}
	if got := names(sum.Entries, outcome(report.Kept)); !slices.Equal(got, sum.Skipped) {
		t.Errorf("Kept entries = %v, Skipped list = %v", got, sum.Skipped)
	}
	created := names(sum.Entries, func(e report.Entry) bool { return e.Kind == report.File && e.Outcome == report.Created })
	if !slices.Equal(created, sum.Created) {
		t.Errorf("file Created entries = %v, Created list = %v", created, sum.Created)
	}
}

func runEntries(t *testing.T, home Home, target string, fr runner.Runner, opts Options) Summary {
	t.Helper()
	opts.Profile, opts.Target = "test", target
	opts.Out = &bytes.Buffer{}
	sum, err := Run(fr, allFound, opts, home)
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

func TestRunEntriesRecordTheHappyPath(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())

	sum := runEntries(t, home, t.TempDir(), &runner.FakeRunner{}, Options{})

	if sum.Profile != "test" {
		t.Errorf("Profile = %q, want the resolved recipe name", sum.Profile)
	}
	assertHasEntries(t, sum,
		report.Entry{Kind: report.Global, Name: "headroom", Outcome: report.Installed},
		report.Entry{Kind: report.Global, Name: "code_graph", Outcome: report.Installed},
		report.Entry{Kind: report.Component, Name: "s", Outcome: report.Installed},
		report.Entry{Kind: report.Command, Name: "graphify update .", Outcome: report.Installed},
		report.Entry{Kind: report.File, Name: ".mcp.json", Outcome: report.Created},
		report.Entry{Kind: report.File, Name: "CLAUDE.md", Outcome: report.Created},
		report.Entry{Kind: report.File, Name: ".claude/settings.json", Outcome: report.Created},
		report.Entry{Kind: report.File, Name: ".gitignore", Outcome: report.Created},
		report.Entry{Kind: report.File, Name: ".claude/.ray-profile", Outcome: report.Created},
		report.Entry{Kind: report.File, Name: store.BaselineFile, Outcome: report.Created},
	)
	assertEntriesMirrorTheLists(t, sum)
}

func TestRunEntriesRecordAMissingComponentWithItsPath(t *testing.T) {
	home := newHome(t)
	writeProfile(t, home.ProfilesDir, testProfile())

	sum := runEntries(t, home, t.TempDir(), &runner.FakeRunner{}, Options{})

	assertHasEntries(t, sum, report.Entry{
		Kind: report.Component, Name: "s", Outcome: report.Failed,
		Detail: "not found at " + filepath.Join(home.ComponentsDir, "s"),
	})
	assertEntriesMirrorTheLists(t, sum)
}

// Repetir o init ai: o componente intocado é recopiado (Installed), o que o
// scaffold já escreveu fica (Kept, "already there") e nada é Created de novo.
func TestRunEntriesRecordARepeatedRun(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	runEntries(t, home, target, &runner.FakeRunner{}, Options{})

	sum := runEntries(t, home, target, &runner.FakeRunner{}, Options{})

	assertHasEntries(t, sum,
		report.Entry{Kind: report.Component, Name: "s", Outcome: report.Installed},
		report.Entry{Kind: report.File, Name: "CLAUDE.md", Outcome: report.Kept, Detail: "already there"},
	)
	if created := names(sum.Entries, func(e report.Entry) bool { return e.Outcome == report.Created }); len(created) != 0 {
		t.Errorf("second run created %v, want nothing", created)
	}
	assertEntriesMirrorTheLists(t, sum)
}

func TestRunEntriesRecordAnEditedComponentAsKeptWithTheReason(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())
	target := t.TempDir()
	runEntries(t, home, target, &runner.FakeRunner{}, Options{})
	skill := filepath.Join(target, ".claude", "skills", "s", "SKILL.md")
	if err := os.WriteFile(skill, []byte("# edited by the user"), 0o644); err != nil {
		t.Fatal(err)
	}

	sum := runEntries(t, home, target, &runner.FakeRunner{}, Options{})

	var got *report.Entry
	for i, e := range sum.Entries {
		if e.Kind == report.Component && e.Name == "s" {
			got = &sum.Entries[i]
		}
	}
	if got == nil || got.Outcome != report.Kept || !strings.HasPrefix(got.Detail, "edited locally") {
		t.Errorf("component entry = %+v, want Kept with the reason starting \"edited locally\"", got)
	}
	assertEntriesMirrorTheLists(t, sum)
}

func TestRunEntriesRecordAFailedGlobalWithItsReason(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())
	fr := &runner.FakeRunner{Results: map[string]runner.Result{
		"uv tool install graphifyy": {ExitCode: 2, Stderr: "error: No space left on device\n"},
	}}

	sum := runEntries(t, home, t.TempDir(), fr, Options{})

	assertHasEntries(t, sum,
		report.Entry{Kind: report.Global, Name: "headroom", Outcome: report.Installed},
		report.Entry{
			Kind: report.Global, Name: "code_graph", Outcome: report.Failed,
			Detail: "`uv tool install graphifyy`: exit 2: error: No space left on device",
		},
	)
	if !sum.HadFailure {
		t.Error("HadFailure = false, want the failed global to fail the run")
	}
	assertEntriesMirrorTheLists(t, sum)
}

// O índice do graphify é regenerável: o comando por projeto que falha é só um
// aviso. Não vira entrada e não derruba a execução.
func TestRunEntriesSkipAPerProjectCommandThatFailed(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())
	fr := &runner.FakeRunner{Results: map[string]runner.Result{"graphify update .": {ExitCode: 1}}}

	sum := runEntries(t, home, t.TempDir(), fr, Options{NoGlobal: true})

	if cmds := names(sum.Entries, func(e report.Entry) bool { return e.Kind == report.Command }); len(cmds) != 0 {
		t.Errorf("command entries = %v, want none for a command that failed", cmds)
	}
	if failed := names(sum.Entries, func(e report.Entry) bool { return e.Outcome == report.Failed }); len(failed) != 0 || sum.HadFailure {
		t.Errorf("Failed entries = %v, HadFailure = %v; a failed per-project command is a warning only", failed, sum.HadFailure)
	}
	if !slices.ContainsFunc(sum.Warnings, func(w string) bool { return strings.Contains(w, "graphify update .") }) {
		t.Errorf("Warnings = %v, want the failed command to stay a warning", sum.Warnings)
	}
}

func TestRunEntriesInDryRunAreWhatWouldBeDone(t *testing.T) {
	home := newHome(t)
	seedComponent(t, home, "s")
	writeProfile(t, home.ProfilesDir, testProfile())

	sum := runEntries(t, home, t.TempDir(), &runner.FakeRunner{}, Options{DryRun: true})

	assertHasEntries(t, sum,
		report.Entry{Kind: report.Component, Name: "s", Outcome: report.Installed},
		report.Entry{Kind: report.File, Name: "CLAUDE.md", Outcome: report.Created},
	)
	if sum.Profile != "test" {
		t.Errorf("Profile = %q, want the resolved recipe name", sum.Profile)
	}
	assertEntriesMirrorTheLists(t, sum)
}
