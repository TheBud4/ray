package status

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTarget monta um projeto com ambiente ray mínimo e devolve o caminho.
func newTarget(t *testing.T, skills, agents, commands []string) string {
	t.Helper()
	target := t.TempDir()
	for dir, names := range map[string][]string{
		"skills": skills, "agents": agents, "commands": commands,
	} {
		for _, n := range names {
			p := filepath.Join(target, ".claude", dir, n)
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte("x\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return target
}

func TestRunReportsNoEnvironmentWhenClaudeDirMissing(t *testing.T) {
	rep, err := Run(nil, Options{Target: t.TempDir()}, Home{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if len(rep.Problems) != 1 {
		t.Fatalf("Problems = %v, want exactly one", rep.Problems)
	}
	if !strings.Contains(rep.Problems[0], "no ray environment") {
		t.Errorf("Problems[0] = %q, want it to name the missing environment", rep.Problems[0])
	}
	// Sem ambiente, nenhuma outra checagem deve ter rodado e inventado dado.
	if rep.Inventory != (Inventory{}) {
		t.Errorf("Inventory = %+v, want the zero value", rep.Inventory)
	}
}

func TestRunCountsInventory(t *testing.T) {
	target := newTarget(t, []string{"tdd/SKILL.md", "brainstorm/SKILL.md"},
		[]string{"reviewer.md"}, []string{"destilar.md", "revisar.md", "handoff.md"})

	rep, err := Run(nil, Options{Target: target}, Home{})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	want := Inventory{Skills: 2, Agents: 1, Commands: 3}
	if rep.Inventory != want {
		t.Errorf("Inventory = %+v, want %+v", rep.Inventory, want)
	}
	if len(rep.Problems) != 0 {
		t.Errorf("Problems = %v, want none for a healthy environment", rep.Problems)
	}
}
