package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/profile"
)

func TestProfileShowChecksTheRecipeAndSaysWhetherItIsReady(t *testing.T) {
	home := dryRunHome(t)
	comps := filepath.Join(home, "components")
	if err := os.MkdirAll(filepath.Join(comps, "good"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestProfile(t, filepath.Join(home, "profiles"), &profile.Profile{
		Name:       "ok",
		Components: []profile.Component{{Name: "good", Dest: ".claude/skills"}},
		Scaffold:   profile.Scaffold{Files: []profile.ScaffoldFile{{Path: "CLAUDE.md"}}},
	})
	out, err := execRoot(t, "profile", "show", "ok")
	if err != nil || !strings.Contains(out, "good -> "+filepath.Join(".claude/skills", "good")+"  ok") || !strings.Contains(out, "CLAUDE.md <- CLAUDE.md.tmpl") || !strings.HasSuffix(strings.TrimSpace(out), "ready") {
		t.Errorf("ready profile: %q, %v", out, err)
	}

	writeTestProfile(t, filepath.Join(home, "profiles"), &profile.Profile{
		Name:       "bad",
		Components: []profile.Component{{Name: "gone", Dest: ".claude/skills"}},
		Scaffold:   profile.Scaffold{Files: []profile.ScaffoldFile{{Path: "x.md", Template: "nope.tmpl"}}},
	})
	out, _ = execRoot(t, "profile", "show", "bad")
	for _, want := range []string{"missing: not found at", "missing template", "2 problems: `ray init ai --profile bad` would refuse or skip"} {
		if !strings.Contains(out, want) {
			t.Errorf("bad profile: missing %q in\n%s", want, out)
		}
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
