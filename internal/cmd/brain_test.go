package cmd

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/rayconfig"
	"github.com/TheBud4/ray/internal/runner"
)

func TestRunBrainSetPersistsExistingDir(t *testing.T) {
	base := t.TempDir()
	configPath := filepath.Join(base, "config.yaml")
	brain := filepath.Join(base, "MegaBrain")
	if err := os.Mkdir(brain, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := runBrainSet(configPath, brain, io.Discard); err != nil {
		t.Fatalf("runBrainSet() error = %v", err)
	}

	cfg, err := rayconfig.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Brain != brain {
		t.Errorf("Brain = %q, want %q", cfg.Brain, brain)
	}
}

// Sucesso mudo num comando que grava configuração deixa a pessoa sem saber se
// pegou: ela roda `ray brain status` em seguida só para conferir. Confirmar
// nomeando o caminho fecha isso numa linha.
func TestRunBrainSetConfirmsNamingThePath(t *testing.T) {
	base := t.TempDir()
	brain := filepath.Join(base, "MegaBrain")
	if err := os.Mkdir(brain, 0o755); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runBrainSet(filepath.Join(base, "config.yaml"), brain, &out); err != nil {
		t.Fatalf("runBrainSet() error = %v", err)
	}

	if !strings.Contains(out.String(), brain) {
		t.Errorf("output = %q, want it to confirm by naming %q", out.String(), brain)
	}
}

func TestRunBrainSetRejectsMissingPathAndWritesNothing(t *testing.T) {
	base := t.TempDir()
	configPath := filepath.Join(base, "config.yaml")
	missing := filepath.Join(base, "nao-existe")

	if err := runBrainSet(configPath, missing, io.Discard); err == nil {
		t.Fatal("runBrainSet() = nil for a missing path, want error")
	}

	// `ray brain set` nunca cria a vault — o usuário é dono dela.
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Errorf("runBrainSet() created %s, it must never do so", missing)
	}
	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		t.Errorf("runBrainSet() wrote config despite failing validation")
	}
}

func TestRunBrainSetRejectsFile(t *testing.T) {
	base := t.TempDir()
	file := filepath.Join(base, "nota.md")
	if err := os.WriteFile(file, []byte("# x"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := runBrainSet(filepath.Join(base, "config.yaml"), file, io.Discard); err == nil {
		t.Fatal("runBrainSet() = nil for a regular file, want error")
	}
}

func TestRunBrainStatusUnconfigured(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")

	var out bytes.Buffer
	if err := runBrainStatus(configPath, &out); err != nil {
		t.Fatalf("runBrainStatus() error = %v", err)
	}
	if !strings.Contains(out.String(), "not configured") {
		t.Errorf("output = %q, want a 'not configured' hint", out.String())
	}
}

func TestRunBrainStatusCountsNotes(t *testing.T) {
	base := t.TempDir()
	configPath := filepath.Join(base, "config.yaml")
	brain := filepath.Join(base, "MegaBrain")
	if err := os.MkdirAll(filepath.Join(brain, "Notas"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(brain, "Notas", "Backend.md"), []byte("# b"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := runBrainSet(configPath, brain, io.Discard); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runBrainStatus(configPath, &out); err != nil {
		t.Fatalf("runBrainStatus() error = %v", err)
	}
	got := out.String()
	if !strings.Contains(got, brain) {
		t.Errorf("output = %q, want the brain path", got)
	}
	if !strings.Contains(got, "markdown files: 1") {
		t.Errorf("output = %q, want the note count", got)
	}
}

func TestResolveBrainPathErrorsWhenUnset(t *testing.T) {
	t.Setenv("RAY_BRAIN", "")
	configPath := filepath.Join(t.TempDir(), "config.yaml")

	if _, err := resolveBrainPath(configPath); err == nil {
		t.Fatal("resolveBrainPath() = nil error, want failure when unconfigured")
	}
}

func TestResolveBrainPathPrefersEnv(t *testing.T) {
	base := t.TempDir()
	configPath := filepath.Join(base, "config.yaml")
	brain := filepath.Join(base, "MegaBrain")
	if err := os.Mkdir(brain, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runBrainSet(configPath, brain, io.Discard); err != nil {
		t.Fatal(err)
	}

	t.Setenv("RAY_BRAIN", "/from/env")
	got, err := resolveBrainPath(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if got != "/from/env" {
		t.Errorf("resolveBrainPath() = %q, want the RAY_BRAIN override", got)
	}
}

func TestRunBrainOpenInvokesOpener(t *testing.T) {
	base := t.TempDir()
	configPath := filepath.Join(base, "config.yaml")
	brain := filepath.Join(base, "MegaBrain")
	if err := os.Mkdir(brain, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := runBrainSet(configPath, brain, io.Discard); err != nil {
		t.Fatal(err)
	}

	t.Setenv("RAY_BRAIN", "")
	fake := &runner.FakeRunner{}
	if err := runBrainOpen(fake, configPath); err != nil {
		t.Fatalf("runBrainOpen() error = %v", err)
	}
	if len(fake.Calls) == 0 {
		t.Fatal("runBrainOpen() ran no command")
	}
	if !strings.Contains(strings.Join(fake.Calls[0].Args, " "), brain) {
		t.Errorf("opened %v, want a command mentioning %q", fake.Calls[0], brain)
	}
}
