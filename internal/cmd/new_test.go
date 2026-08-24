package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/TheBud4/ray/internal/initai"
	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/runner"
)

var allFound = stubLooker{
	"npx": true, "node": true, "python3.10+": true,
	"uv": true, "headroom": true, "graphify": true,
}

// newTestProfile propositalmente não traz Components: estes testes exercem
// `ray new` (create step + git init + scaffold), não a cópia local de
// componente (coberta em internal/initai).
func newTestProfile(create []string) *profile.Profile {
	return &profile.Profile{
		Name:     "test",
		Create:   create,
		Scaffold: profile.Scaffold{Files: []profile.ScaffoldFile{{Path: "CLAUDE.md"}}},
	}
}

func writeTestProfile(t *testing.T, profilesDir string, p *profile.Profile) {
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

func newTestHome(t *testing.T) initai.Home {
	t.Helper()
	base := t.TempDir()
	return initai.Home{
		ProfilesDir:  filepath.Join(base, "profiles"),
		TemplatesDir: filepath.Join(base, "templates"),
		ConfigPath:   filepath.Join(base, "config.yaml"),
		StatePath:    filepath.Join(base, "state.yaml"),
		StoreDir:     filepath.Join(base, "store"),
	}
}

func TestRunNewCreatesGitInitsAndRunsInitAI(t *testing.T) {
	sandbox := t.TempDir()
	t.Chdir(sandbox)

	home := newTestHome(t)
	writeTestProfile(t, home.ProfilesDir, newTestProfile([]string{"echo {{.Name}}"}))

	fr := &runner.FakeRunner{}
	initOpts := initai.Options{}

	sum, err := runNew(fr, allFound, home.ProfilesDir, "test", "myproj", false, false, initOpts, home)
	if err != nil {
		t.Fatalf("runNew() error = %v", err)
	}
	if sum.HadFailure {
		t.Fatalf("HadFailure = true, Failed = %v", sum.Failed)
	}

	if _, err := os.Stat(filepath.Join(sandbox, "myproj")); err != nil {
		t.Fatalf("target dir not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(sandbox, "myproj", "CLAUDE.md")); err != nil {
		t.Fatalf("initai.Run did not scaffold the project: %v", err)
	}

	foundCreate, foundGit := false, false
	for _, c := range fr.Calls {
		if c.String() == "echo myproj" {
			foundCreate = true
		}
		if c.String() == "git init -q" {
			foundGit = true
		}
	}
	if !foundCreate {
		t.Errorf("Calls = %v, want the rendered create step to have run", fr.Calls)
	}
	if !foundGit {
		t.Errorf("Calls = %v, want `git init -q` to have run", fr.Calls)
	}
}

// Simular um `ray new` não pode deixar a pasta do projeto para trás: o
// MkdirAll não passa pelo runner, então o --dry-run não o alcança sozinho.
func TestRunNewDryRunCreatesNothing(t *testing.T) {
	sandbox := t.TempDir()
	t.Chdir(sandbox)

	home := newTestHome(t)
	writeTestProfile(t, home.ProfilesDir, newTestProfile([]string{"echo {{.Name}}"}))

	fr := &runner.FakeRunner{}
	initOpts := initai.Options{DryRun: true, Out: &bytes.Buffer{}}

	if _, err := runNew(fr, allFound, home.ProfilesDir, "test", "myproj", false, true, initOpts, home); err != nil {
		t.Fatalf("runNew() error = %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(sandbox, "myproj")); !os.IsNotExist(statErr) {
		t.Errorf("stat(myproj) err = %v, want IsNotExist — dry-run must not create the project dir", statErr)
	}
}

func TestRunNewNoGitSkipsGitInit(t *testing.T) {
	sandbox := t.TempDir()
	t.Chdir(sandbox)

	home := newTestHome(t)
	writeTestProfile(t, home.ProfilesDir, newTestProfile(nil))

	fr := &runner.FakeRunner{}
	initOpts := initai.Options{}

	_, err := runNew(fr, allFound, home.ProfilesDir, "test", "myproj", true, false, initOpts, home)
	if err != nil {
		t.Fatalf("runNew() error = %v", err)
	}
	for _, c := range fr.Calls {
		if c.String() == "git init -q" {
			t.Fatalf("git init ran despite --no-git: %v", fr.Calls)
		}
	}
}

func TestRunNewRejectsExistingNonEmptyTarget(t *testing.T) {
	sandbox := t.TempDir()
	t.Chdir(sandbox)
	if err := os.MkdirAll("myproj", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("myproj", "existing.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	home := newTestHome(t)
	writeTestProfile(t, home.ProfilesDir, newTestProfile(nil))

	_, err := runNew(&runner.FakeRunner{}, allFound, home.ProfilesDir, "test", "myproj", false, false, initai.Options{}, home)
	if err == nil {
		t.Fatal("runNew() = nil error, want error for a non-empty existing target")
	}
}

func TestRunNewAbortsOnFailedCreateStepBeforeGitOrInitAI(t *testing.T) {
	sandbox := t.TempDir()
	t.Chdir(sandbox)

	home := newTestHome(t)
	writeTestProfile(t, home.ProfilesDir, newTestProfile([]string{"false-cmd {{.Name}}"}))

	fr := &runner.FakeRunner{Results: map[string]runner.Result{
		"false-cmd myproj": {ExitCode: 1},
	}}

	_, err := runNew(fr, allFound, home.ProfilesDir, "test", "myproj", false, false, initai.Options{}, home)
	if err == nil {
		t.Fatal("runNew() = nil error, want error when a create step fails")
	}
	for _, c := range fr.Calls {
		if c.String() == "git init -q" {
			t.Fatal("git init ran despite the create step failing first")
		}
	}
	if _, statErr := os.Stat(filepath.Join(sandbox, "myproj", "CLAUDE.md")); !os.IsNotExist(statErr) {
		t.Error("CLAUDE.md should not exist: initai.Run must not run after a failed create step")
	}
}
