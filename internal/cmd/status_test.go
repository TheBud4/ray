package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/status"
)

func TestPrintStatusHealthyEnvironmentShowsNoWarnings(t *testing.T) {
	var out bytes.Buffer
	printStatus(&out, status.Report{
		Profile:   "go",
		Inventory: status.Inventory{Skills: 4, Agents: 2, MCPServers: 3},
		Git:       status.GitClean,
	})
	got := out.String()
	if strings.Contains(got, "⚠") {
		t.Errorf("status = %q, want no warning marker for a healthy environment", got)
	}
	for _, want := range []string{"profile: go", "4 skills", "3 MCP servers"} {
		if !strings.Contains(got, want) {
			t.Errorf("status = %q, want it to contain %q", got, want)
		}
	}
}

// A linha de fatos concorda com o número que ela mesma imprime. Compara a
// linha inteira porque "1 skill" é prefixo de "1 skills": asserção de
// substring passaria com o plural errado.
func TestPrintFactsAgreesWithTheCount(t *testing.T) {
	cases := []struct {
		name string
		inv  status.Inventory
		want string
	}{
		{
			"one of each",
			status.Inventory{Skills: 1, Agents: 1, Commands: 1, MCPServers: 1},
			"profile: go · 1 skill · 1 agent · 1 command · 1 MCP server",
		},
		{
			"more than one",
			status.Inventory{Skills: 4, Agents: 2, Commands: 3, MCPServers: 2},
			"profile: go · 4 skills · 2 agents · 3 commands · 2 MCP servers",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			printFacts(&out, "go", tc.inv)
			if got := strings.TrimRight(out.String(), "\n"); got != tc.want {
				t.Errorf("facts = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPrintStatusNeverTrackedIsANoteNotAWarning(t *testing.T) {
	var out bytes.Buffer
	printStatus(&out, status.Report{
		Profile:  "go",
		Git:      status.GitNeverTracked,
		AddPaths: []string{".claude", ".mcp.json"},
	})
	got := out.String()
	if strings.Contains(got, "⚠") {
		t.Errorf("status = %q, want no warning: never-tracked is the normal state after init", got)
	}
	if !strings.Contains(got, "git add .claude .mcp.json") {
		t.Errorf("status = %q, want the git add line", got)
	}
	for _, forbidden := range []string{"git add -A", "git add --all", "git add .\n"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("status suggests blind %q, which guard-add.sh warns against", forbidden)
		}
	}
}

func TestPrintStatusRendersProblemsAndForks(t *testing.T) {
	var out bytes.Buffer
	printStatus(&out, status.Report{
		Profile:  "go",
		Git:      status.GitDirty,
		DirtyN:   2,
		Problems: []string{".gitignore: the ray block is missing !.claude/skills/"},
		Forks: []status.ComponentState{
			{Coord: "skills:o/r#tdd", State: status.ForkEdited},
			{Coord: "skills:o/r#brainstorm", State: status.ForkPristine},
		},
	})
	got := out.String()
	for _, want := range []string{"2 changed", "!.claude/skills/", "skills:o/r#tdd", "edited locally"} {
		if !strings.Contains(got, want) {
			t.Errorf("status = %q, want it to contain %q", got, want)
		}
	}
	// Componente intocado não é notícia — não polui a saída.
	if strings.Contains(got, "brainstorm") {
		t.Errorf("status = %q, want pristine components left out", got)
	}
}

// A asserção que trava a decisão de exit code contra regressão.
func TestRunStatusReturnsNilEvenWithProblems(t *testing.T) {
	target := t.TempDir() // sem .claude/ → garante ao menos um problema
	if err := runStatus(target, &bytes.Buffer{}); err != nil {
		t.Errorf("runStatus() error = %v, want nil: a detected problem is not a command failure", err)
	}
}
