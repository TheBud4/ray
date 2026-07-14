package runfile

import (
	"os"
	"path/filepath"
	"testing"
)

func writeYAML(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadGlobalOnly(t *testing.T) {
	workdir := t.TempDir()
	globalPath := filepath.Join(t.TempDir(), "commands.yaml")
	writeYAML(t, globalPath, `
commands:
  test:
    description: run tests
    steps: ["go test ./..."]
`)

	got, err := Load(workdir, globalPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	r, ok := got["test"]
	if !ok {
		t.Fatal("expected \"test\" alias to be resolved")
	}
	if r.Source != SourceGlobal {
		t.Errorf("Source = %q, want %q", r.Source, SourceGlobal)
	}
	wantBaseDir, err := filepath.Abs(workdir)
	if err != nil {
		t.Fatal(err)
	}
	if r.BaseDir != wantBaseDir {
		t.Errorf("BaseDir = %q, want %q", r.BaseDir, wantBaseDir)
	}
	if len(r.Steps) != 1 || r.Steps[0] != "go test ./..." {
		t.Errorf("Steps = %v, want [\"go test ./...\"]", r.Steps)
	}
}

func TestLoadProjectOverridesGlobal(t *testing.T) {
	workdir := t.TempDir()
	globalPath := filepath.Join(t.TempDir(), "commands.yaml")
	writeYAML(t, globalPath, `
commands:
  test:
    description: global test
    steps: ["echo global"]
`)
	writeYAML(t, filepath.Join(workdir, "ray.yaml"), `
commands:
  test:
    description: project test
    steps: ["echo project"]
`)

	got, err := Load(workdir, globalPath)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	r, ok := got["test"]
	if !ok {
		t.Fatal("expected \"test\" alias to be resolved")
	}
	if r.Source != SourceProject {
		t.Errorf("Source = %q, want %q", r.Source, SourceProject)
	}
	if r.Description != "project test" {
		t.Errorf("Description = %q, want project test to win", r.Description)
	}
	wantBaseDir, err := filepath.Abs(workdir)
	if err != nil {
		t.Fatal(err)
	}
	if r.BaseDir != wantBaseDir {
		t.Errorf("BaseDir = %q, want %q", r.BaseDir, wantBaseDir)
	}
}

func TestFindProjectFileWalksUpTheTree(t *testing.T) {
	root := t.TempDir()
	writeYAML(t, filepath.Join(root, "ray.yaml"), `
commands:
  lint:
    description: lint
    steps: ["echo lint"]
`)
	sub := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := Load(sub, filepath.Join(t.TempDir(), "missing-commands.yaml"))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	r, ok := got["lint"]
	if !ok {
		t.Fatal("expected \"lint\" alias to be found by walking up the tree")
	}
	wantBaseDir, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if r.BaseDir != wantBaseDir {
		t.Errorf("BaseDir = %q, want the ancestor %q (not workdir)", r.BaseDir, wantBaseDir)
	}
}

func TestLoadBothMissingIsEmptyNotError(t *testing.T) {
	workdir := t.TempDir()
	globalPath := filepath.Join(t.TempDir(), "commands.yaml")

	got, err := Load(workdir, globalPath)
	if err != nil {
		t.Fatalf("Load() error = %v, want nil when both files are missing", err)
	}
	if len(got) != 0 {
		t.Errorf("Load() = %v, want empty map", got)
	}
}

func TestLoadMalformedYAMLErrors(t *testing.T) {
	workdir := t.TempDir()
	globalPath := filepath.Join(t.TempDir(), "commands.yaml")
	writeYAML(t, globalPath, ":\n  - [")

	if _, err := Load(workdir, globalPath); err == nil {
		t.Fatal("Load() = nil error, want error for malformed YAML")
	}
}
