package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/TheBud4/ray/internal/initai"
	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/runner"
	"github.com/TheBud4/ray/internal/scaffold"
)

func resetInitAIFlags(t *testing.T) {
	t.Helper()
	prevProfile, prevMode := flagProfile, flagMode
	prevGlobal, prevForce := flagGlobal, flagForce
	prevNoGlobal, prevReinstall := flagNoGlobal, flagReinstallGlobal
	prevDryRun := flagDryRun
	t.Cleanup(func() {
		flagProfile, flagMode = prevProfile, prevMode
		flagGlobal, flagForce = prevGlobal, prevForce
		flagNoGlobal, flagReinstallGlobal = prevNoGlobal, prevReinstall
		flagDryRun = prevDryRun
	})
}

func TestBuildInitAIOptionsMapsFlags(t *testing.T) {
	resetInitAIFlags(t)
	flagProfile = "go"
	flagMode = scaffold.ModeLearn
	flagGlobal = true
	flagForce = true
	flagNoGlobal = true
	flagReinstallGlobal = true
	flagDryRun = true

	opts := buildInitAIOptions("/tmp/project", &bytes.Buffer{})

	if opts.Profile != "go" || opts.Mode != scaffold.ModeLearn || opts.Target != "/tmp/project" {
		t.Fatalf("opts = %+v, want Profile=go Mode=learn Target=/tmp/project", opts)
	}
	if !opts.Global || !opts.Force || !opts.NoGlobal || !opts.ReinstallGlobal || !opts.DryRun {
		t.Fatalf("opts = %+v, want all bool flags true", opts)
	}
}

func TestRunInitAIPrintsSummaryAndErrorsOnFailure(t *testing.T) {
	base := t.TempDir()
	profilesDir := filepath.Join(base, "profiles")
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	prof := &profile.Profile{
		Name:       "test",
		Components: []profile.Component{{Via: profile.ViaSkills, Skill: "s", Source: "o/r"}},
		Scaffold:   profile.Scaffold{Files: []profile.ScaffoldFile{{Path: "CLAUDE.md"}}},
	}
	data, err := yaml.Marshal(prof)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, "test.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	home := initai.Home{
		ProfilesDir:  profilesDir,
		TemplatesDir: filepath.Join(base, "templates"),
		VaultDir:     filepath.Join(base, "vault"),
		ConfigPath:   filepath.Join(base, "config.yaml"),
		StatePath:    filepath.Join(base, "state.yaml"),
	}
	target := t.TempDir()

	l := stubLooker{"npx": true, "node": true}
	fr := &runner.FakeRunner{Results: map[string]runner.Result{
		"npx skills add o/r --skill s -a claude-code -y": {ExitCode: 1},
	}}
	var out bytes.Buffer
	opts := initai.Options{Profile: "test", Target: target, Mode: scaffold.ModeBuild, Out: &out}

	err = runInitAI(fr, l, opts, home, &out)
	if err == nil {
		t.Fatal("runInitAI() = nil error, want error when a component fails")
	}
	if !strings.Contains(out.String(), "Failed") {
		t.Errorf("output = %q, want it to contain the Failed summary section", out.String())
	}
}
