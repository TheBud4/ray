package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/ui"
)

// fixHome fixa a pasta home que o cmd abrevia em "~" na exibição. O
// t.TempDir() do Windows fica dentro do %USERPROFILE%, e viraria "~\AppData\…".
func fixHome(t *testing.T, home string) {
	t.Helper()
	prev := userHome
	userHome = func() string { return home }
	t.Cleanup(func() { userHome = prev })
}

func TestProfileShowChecksTheRecipeAndSaysWhetherItIsReady(t *testing.T) {
	home := dryRunHome(t)
	fixHome(t, "")
	comps := filepath.Join(home, "components")
	if err := os.MkdirAll(filepath.Join(comps, "good"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestProfile(t, filepath.Join(home, "profiles"), &profile.Profile{
		Name:        "ok",
		Description: "a sound recipe",
		Components:  []profile.Component{{Name: "good", Dest: ".claude/skills"}},
		Scaffold:    profile.Scaffold{Files: []profile.ScaffoldFile{{Path: "CLAUDE.md"}}},
	})
	out, err := execRoot(t, "profile", "show", "ok")
	if err != nil {
		t.Fatal(err)
	}
	assertNoEscape(t, out)
	head := "ok · a sound recipe\n\nComponents\n  ✓ good  -> " + filepath.Join(".claude/skills", "good") + "\n\nFiles\n"
	if !strings.HasPrefix(out, head) {
		t.Errorf("output starts with\n%q\nwant\n%q", out, head)
	}
	if !regexp.MustCompile(`\n  ✓ CLAUDE\.md +<- CLAUDE\.md\.tmpl  \(new\)\n`).MatchString(out) {
		t.Errorf("the CLAUDE.md file line is missing or misaligned in\n%s", out)
	}
	if !strings.HasSuffix(out, "\n\n✓ ready\n") {
		t.Errorf("a sound recipe ends with the ready verdict, got\n%s", out)
	}
	if strings.Contains(out, "Globals") || strings.Contains(out, "MCP servers") {
		t.Errorf("a recipe without integrations has no Globals or MCP servers section:\n%s", out)
	}
}

func TestProfileShowListsEveryKindOfProblem(t *testing.T) {
	home := dryRunHome(t)
	fixHome(t, "")
	comps := filepath.Join(home, "components")
	if err := os.MkdirAll(filepath.Join(comps, "good"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestProfile(t, filepath.Join(home, "profiles"), &profile.Profile{
		Name:         "bad",
		Description:  "a recipe with every kind of problem",
		Components:   []profile.Component{{Name: "good", Dest: ".claude/skills"}, {Name: "gone", Dest: ".claude/skills"}},
		Scaffold:     profile.Scaffold{Files: []profile.ScaffoldFile{{Path: "a.md", Template: "nope.tmpl"}, {Path: "CLAUDE.md"}}},
		Integrations: profile.Integrations{Headroom: true},
	})
	out, err := execRoot(t, "profile", "show", "bad")
	if err != nil {
		t.Fatal(err)
	}
	assertNoEscape(t, out)
	head := "bad · a recipe with every kind of problem\n\n" +
		"Components\n" +
		"  ✓ good  -> " + filepath.Join(".claude/skills", "good") + "\n" +
		"  ✗ gone  not found at " + filepath.Join(comps, "gone") + "\n\n" +
		"Files\n"
	if !strings.HasPrefix(out, head) {
		t.Errorf("output starts with\n%q\nwant\n%q", out, head)
	}
	if !regexp.MustCompile(`\n  ✗ a\.md +<- nope\.tmpl  missing template\n`).MatchString(out) {
		t.Errorf("the problem file line is missing or misaligned in\n%s", out)
	}
	tail := "\n\nGlobals (install-once)\n  [headroom] uv tool install headroom-ai[mcp]\n\n" +
		"MCP servers\n  headroom: headroom mcp\n\n" +
		"✗ 2 problems — `ray init ai --profile bad` would refuse or skip them\n"
	if !strings.HasSuffix(out, tail) {
		t.Errorf("output ends with\n%q\nwant\n%q", out, tail)
	}
}

func TestProfileShowVerdictIsSingularForOneProblem(t *testing.T) {
	home := dryRunHome(t)
	fixHome(t, "")
	writeTestProfile(t, filepath.Join(home, "profiles"), &profile.Profile{
		Name:       "one",
		Components: []profile.Component{{Name: "gone", Dest: ".claude/skills"}},
	})
	out, err := execRoot(t, "profile", "show", "one")
	if err != nil {
		t.Fatal(err)
	}
	if want := "✗ 1 problem — `ray init ai --profile one` would refuse or skip them\n"; !strings.HasSuffix(out, want) {
		t.Errorf("output ends with %q, want %q", out[max(0, len(out)-80):], want)
	}
}

// O caminho do componente ausente é exibido com a home em "~"; o domínio guarda
// o caminho inteiro.
func TestProfileShowAbbreviatesTheHomeInTheMissingComponentPath(t *testing.T) {
	home := dryRunHome(t)
	fixHome(t, home)
	writeTestProfile(t, filepath.Join(home, "profiles"), &profile.Profile{
		Name:       "one",
		Components: []profile.Component{{Name: "gone", Dest: ".claude/skills"}},
	})
	out, err := execRoot(t, "profile", "show", "one")
	if err != nil {
		t.Fatal(err)
	}
	if want := "not found at " + filepath.Join("~", "components", "gone") + "\n"; !strings.Contains(out, want) {
		t.Errorf("output lacks %q:\n%s", want, out)
	}
	if strings.Contains(out, home) {
		t.Errorf("the home directory %q was not abbreviated:\n%s", home, out)
	}
}

func TestProfileShowColorsTheMarkersTheTitleAndTheVerdict(t *testing.T) {
	home := dryRunHome(t)
	fixHome(t, "")
	comps := filepath.Join(home, "components")
	if err := os.MkdirAll(filepath.Join(comps, "good"), 0o755); err != nil {
		t.Fatal(err)
	}
	profiles := filepath.Join(home, "profiles")
	writeTestProfile(t, profiles, &profile.Profile{
		Name:        "bad",
		Description: "broken",
		Components:  []profile.Component{{Name: "good", Dest: ".claude/skills"}, {Name: "gone", Dest: ".claude/skills"}},
	})
	writeTestProfile(t, profiles, &profile.Profile{Name: "ok", Description: "sound"})

	show := func(name string) string {
		t.Helper()
		p, err := profile.LoadByName(profiles, name)
		if err != nil {
			t.Fatal(err)
		}
		rep, err := templatesFor(t).Inspect(p, comps)
		if err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if err := runProfileShowWith(profiles, name, rep, ui.New(&out, true)); err != nil {
			t.Fatal(err)
		}
		return out.String()
	}

	bad := show("bad")
	for _, want := range []string{
		esc + "[1mbad" + esc + "[22m " + esc + "[90m· broken" + esc + "[0m\n",
		esc + "[1mComponents" + esc + "[22m\n",
		"  " + esc + "[32m✓" + esc + "[0m good  " + esc + "[90m-> ",
		"  " + esc + "[31m✗" + esc + "[0m " + esc + "[31mgone" + esc + "[0m  " + esc + "[90mnot found at ",
		esc + "[31m✗ 1 problem" + esc + "[0m " + esc + "[90m— `ray init ai --profile bad` would refuse or skip them" + esc + "[0m\n",
	} {
		if !strings.Contains(bad, want) {
			t.Errorf("colored bad profile lacks %q in\n%q", want, bad)
		}
	}
	if want := esc + "[32m✓ ready" + esc + "[0m\n"; !strings.HasSuffix(show("ok"), want) {
		t.Errorf("colored sound profile must end with %q, got %q", want, show("ok"))
	}
}

func TestProfileShowRefusesAnUnreadableBaseline(t *testing.T) {
	home := dryRunHome(t)
	writeTestProfile(t, filepath.Join(home, "profiles"), newTestProfile(nil))
	if err := os.MkdirAll(filepath.Join(home, "store"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "store", "pristine.yaml"), []byte("::: not yaml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := execRoot(t, "profile", "show", "test"); err == nil || !strings.Contains(err.Error(), "pristine.yaml") {
		t.Errorf("error = %v, want the unreadable-baseline refusal", err)
	}
}
