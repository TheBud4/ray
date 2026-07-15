package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/runner"
)

func writeLearnTestProfile(t *testing.T, profilesDir string, p *profile.Profile) {
	t.Helper()
	if err := os.MkdirAll(profilesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	data, err := yaml.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(profilesDir, p.Name+".yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunLearnCheckNoMilestones(t *testing.T) {
	base := t.TempDir()
	profilesDir := filepath.Join(base, "profiles")
	writeLearnTestProfile(t, profilesDir, &profile.Profile{Name: "test"})
	target := t.TempDir()

	out := &bytes.Buffer{}
	if err := runLearnCheck(&runner.FakeRunner{}, profilesDir, target, "test", out); err != nil {
		t.Fatalf("runLearnCheck() error = %v", err)
	}
	if !strings.Contains(out.String(), "no milestones to check") {
		t.Errorf("output = %q, want it to mention no milestones", out.String())
	}
}

func TestRunLearnCheckPasses(t *testing.T) {
	base := t.TempDir()
	profilesDir := filepath.Join(base, "profiles")
	prof := &profile.Profile{
		Name:       "test",
		Milestones: []profile.Milestone{{Goal: "Skeleton compiles", Verify: "true"}},
	}
	writeLearnTestProfile(t, profilesDir, prof)
	target := t.TempDir()

	fr := &runner.FakeRunner{Results: map[string]runner.Result{"true": {ExitCode: 0}}}
	out := &bytes.Buffer{}
	if err := runLearnCheck(fr, profilesDir, target, "test", out); err != nil {
		t.Fatalf("runLearnCheck() error = %v", err)
	}
	if !strings.Contains(out.String(), "milestone passed: Skeleton compiles") {
		t.Errorf("output = %q, want it to confirm the milestone passed", out.String())
	}
}

func TestRunLearnCheckFails(t *testing.T) {
	base := t.TempDir()
	profilesDir := filepath.Join(base, "profiles")
	prof := &profile.Profile{
		Name:       "test",
		Milestones: []profile.Milestone{{Goal: "Skeleton compiles", Verify: "true"}},
	}
	writeLearnTestProfile(t, profilesDir, prof)
	target := t.TempDir()

	fr := &runner.FakeRunner{Results: map[string]runner.Result{"true": {ExitCode: 1, Stderr: "nope"}}}
	out := &bytes.Buffer{}
	err := runLearnCheck(fr, profilesDir, target, "test", out)
	if err == nil {
		t.Fatal("runLearnCheck() = nil error, want error when the milestone fails")
	}
	if !strings.Contains(out.String(), "nope") {
		t.Errorf("output = %q, want it to include the verify failure output", out.String())
	}
}

func TestLearnCmdRegistersCheckSubcommand(t *testing.T) {
	c := newLearnCmd()
	found := false
	for _, sub := range c.Commands() {
		if sub.Use == "check [path]" {
			found = true
		}
	}
	if !found {
		t.Error("newLearnCmd() has no \"check\" subcommand")
	}
}
