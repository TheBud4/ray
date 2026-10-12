package cmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/status"
	"github.com/TheBud4/ray/internal/ui"
)

func printed(rep status.Report) string {
	var out bytes.Buffer
	printStatus(&out, rep)
	return out.String()
}

func TestPrintStatusHealthyEnvironmentShowsNoWarnings(t *testing.T) {
	got := printed(status.Report{
		Profile:   "go",
		Inventory: status.Inventory{Skills: 4, Agents: 2, MCPServers: 3},
		Git:       status.GitClean,
	})
	assertNoEscape(t, got)
	if want := "status · go · 4 skills · 2 agents · 3 MCP servers\n\n✓ all in order\n"; got != want {
		t.Errorf("status = %q, want %q", got, want)
	}
}

// O título nomeia só o que existe: sem perfil, só o inventário; sem nenhum dos
// dois, só "status" — nunca um " · " solto.
func TestPrintStatusTitleNamesOnlyWhatExists(t *testing.T) {
	cases := []struct {
		name string
		rep  status.Report
		want string
	}{
		{"no profile", status.Report{Inventory: status.Inventory{Skills: 2}}, "status · 2 skills\n\n✓ all in order\n"},
		{"nothing", status.Report{}, "status\n\n✓ all in order\n"},
		{"profile only", status.Report{Profile: "go"}, "status · go\n\n✓ all in order\n"},
	}
	for _, c := range cases {
		if got := printed(c.rep); got != c.want {
			t.Errorf("%s: status = %q, want %q", c.name, got, c.want)
		}
	}
}

// A linha de fatos é compartilhada com a tela de abertura: com perfil, o perfil
// é o título e o inventário o detalhe; sem perfil, só o inventário; sem nada,
// nenhuma linha. Compara a linha inteira porque "1 skill" é prefixo de
// "1 skills": asserção de substring passaria com o plural errado.
func TestPrintFactsAgreesWithTheCount(t *testing.T) {
	cases := []struct {
		name    string
		profile string
		inv     status.Inventory
		want    string
	}{
		{
			"one of each", "go",
			status.Inventory{Skills: 1, Agents: 1, Commands: 1, MCPServers: 1},
			"go · 1 skill · 1 agent · 1 command · 1 MCP server\n",
		},
		{
			"more than one", "go",
			status.Inventory{Skills: 4, Agents: 2, Commands: 3, MCPServers: 2},
			"go · 4 skills · 2 agents · 3 commands · 2 MCP servers\n",
		},
		{"without a profile", "", status.Inventory{Skills: 1}, "1 skill\n"},
		{"a profile and nothing else", "go", status.Inventory{}, "go\n"},
		{"nothing at all", "", status.Inventory{}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			printFacts(&out, tc.profile, tc.inv)
			if got := out.String(); got != tc.want {
				t.Errorf("facts = %q, want %q", got, tc.want)
			}
		})
	}
}

// Decisão do dono: o ambiente ainda não versionado é ⚠ e conta no resumo. O
// git add é por nome, nunca cego: o guard-add.sh que o ray instala avisa contra.
func TestPrintStatusNeverTrackedIsAWarning(t *testing.T) {
	got := printed(status.Report{
		Profile:  "go",
		Git:      status.GitNeverTracked,
		AddPaths: []string{".claude", ".mcp.json"},
	})
	assertNoEscape(t, got)
	want := "status · go\n\n⚠ environment not versioned yet\n    git add .claude .mcp.json\n\n1 warning\n"
	if got != want {
		t.Errorf("status = %q, want %q", got, want)
	}
	for _, forbidden := range []string{"git add -A", "git add --all", "git add .\n"} {
		if strings.Contains(got, forbidden) {
			t.Errorf("status suggests blind %q, which guard-add.sh warns against", forbidden)
		}
	}
}

func TestPrintStatusNeverTrackedWithNothingToAddIsNotAWarning(t *testing.T) {
	got := printed(status.Report{Profile: "go", Git: status.GitNeverTracked})
	if want := "status · go\n\n✓ all in order\n"; got != want {
		t.Errorf("status = %q, want %q", got, want)
	}
}

func TestPrintStatusRendersProblemsAndForks(t *testing.T) {
	got := printed(status.Report{
		Profile: "go",
		Git:     status.GitDirty,
		DirtyN:  2,
		// Um problema que traz " — " no texto não é partido em manchete e dica.
		Problems: []string{".gitignore: the ray block is missing !.claude/skills/ — add it back"},
		Forks: []status.ComponentState{
			{Coord: "skills:o/r#tdd", State: status.ForkEdited},
			{Coord: "skills:o/r#brainstorm", State: status.ForkPristine},
			{Coord: "skills:o/r#new", State: status.ForkUnknown},
		},
	})
	assertNoEscape(t, got)
	want := "status · go\n\n" +
		"⚠ .claude/: 2 changed and not committed\n" +
		"⚠ .gitignore: the ray block is missing !.claude/skills/ — add it back\n" +
		"⚠ skills:o/r#tdd: edited locally\n" +
		"    `ray update` will preserve it\n" +
		"⚠ skills:o/r#new: unknown provenance (no pristine baseline)\n\n" +
		"4 warnings\n"
	if got != want {
		t.Errorf("status =\n%q\nwant\n%q", got, want)
	}
	// Componente intocado não é notícia — não polui a saída.
	if strings.Contains(got, "brainstorm") {
		t.Errorf("status = %q, want pristine components left out", got)
	}
}

func TestPrintStatusColorsTheTitleTheWarningsAndTheSummary(t *testing.T) {
	var out bytes.Buffer
	printStatus(ui.New(&out, true), status.Report{
		Profile:  "go",
		Git:      status.GitNeverTracked,
		AddPaths: []string{".claude"},
		Forks:    []status.ComponentState{{Coord: "s", State: status.ForkEdited}},
	})
	want := esc + "[1mstatus" + esc + "[22m " + esc + "[90m· go" + esc + "[0m\n\n" +
		esc + "[33m⚠" + esc + "[0m environment not versioned yet\n" +
		"    " + esc + "[90mgit add .claude" + esc + "[0m\n" +
		esc + "[33m⚠" + esc + "[0m s: edited locally\n" +
		"    " + esc + "[90m`ray update` will preserve it" + esc + "[0m\n\n" +
		esc + "[33m2 warnings" + esc + "[0m\n"
	if out.String() != want {
		t.Errorf("colored status =\n%q\nwant\n%q", out.String(), want)
	}

	out.Reset()
	printStatus(ui.New(&out, true), status.Report{Profile: "go"})
	if want := esc + "[32m✓ all in order" + esc + "[0m\n"; !strings.HasSuffix(out.String(), want) {
		t.Errorf("colored healthy status = %q, want it to end with %q", out.String(), want)
	}
}

// A asserção que trava a decisão de exit code contra regressão.
func TestRunStatusReturnsNilEvenWithProblems(t *testing.T) {
	target := t.TempDir() // sem .claude/ → garante ao menos um problema
	if err := runStatus(target, &bytes.Buffer{}); err != nil {
		t.Errorf("runStatus() error = %v, want nil: a detected problem is not a command failure", err)
	}
}
