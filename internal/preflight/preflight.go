// Package preflight é a fonte única de checagem de dependências externas,
// reaproveitada por `ray doctor` e por `ray init ai`.
package preflight

import "github.com/TheBud4/ray/internal/runner"

// Looker reporta se uma dependência (pelo nome) está disponível no ambiente.
type Looker interface {
	Look(name string) bool
}

// Check é o resultado de uma checagem de dependência.
type Check struct {
	Name     string
	Found    bool
	Required bool
	Hint     string
	Fix      []runner.Command
}

const uvInstallScript = `curl -LsSf https://astral.sh/uv/install.sh | sh`

// Run monta a tabela de checagens (build guide §10) e resolve Found via l.
// needPython liga o requisito de python3.10+/uv (usado por headroom/code_graph).
func Run(l Looker, needPython bool) []Check {
	checks := []Check{
		{Name: "npx", Required: true, Hint: "install Node.js"},
		{Name: "node", Required: false},
		{Name: "jq", Required: false, Hint: "install jq — the warning hooks no-op without it"},
		{Name: "python3.10+", Required: needPython, Hint: "install Python 3.10+"},
		{
			Name: "uv", Required: needPython,
			Fix: []runner.Command{{Name: "sh", Args: []string{"-c", uvInstallScript}}},
		},
		{
			Name: "headroom",
			Fix:  []runner.Command{{Name: "uv", Args: []string{"tool", "install", "headroom-ai[mcp]"}}},
		},
		{
			Name: "graphify",
			Fix:  []runner.Command{{Name: "uv", Args: []string{"tool", "install", "graphifyy"}}},
		},
	}
	for i := range checks {
		checks[i].Found = l.Look(checks[i].Name)
	}
	return checks
}

// MissingRequired devolve os checks obrigatórios que não foram encontrados.
func MissingRequired(checks []Check) []Check {
	var out []Check
	for _, c := range checks {
		if c.Required && !c.Found {
			out = append(out, c)
		}
	}
	return out
}
