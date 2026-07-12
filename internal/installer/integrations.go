package installer

import (
	"github.com/TheBud4/ray/internal/mcp"
	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/runner"
)

// resolveIntegrations acrescenta ao plan as entradas ditadas pelas integrações
// ligadas na receita (tabela §6 do guia).
func resolveIntegrations(in profile.Integrations, opts Options, plan *Plan) {
	if in.Headroom {
		addHeadroom(plan)
	}
	if in.KnowledgeVault {
		addKnowledgeVault(opts, plan)
	}
	if in.UserDocsVault {
		addUserDocsVault(opts, plan)
	}
	if in.SecondBrain {
		addSecondBrain(plan)
	}
	if in.ObsidianFormats {
		addObsidianFormats(plan)
	}
	if in.CodeGraph {
		addCodeGraph(plan)
	}
}

func addHeadroom(plan *Plan) {
	plan.Globals = append(plan.Globals, GlobalStep{
		Key:      "headroom",
		Commands: []runner.Command{{Name: "uv", Args: []string{"tool", "install", "headroom-ai[mcp]"}}},
	})
	plan.Servers = append(plan.Servers, mcp.Server{Name: "headroom", Command: "headroom", Args: []string{"mcp"}})
}

func addKnowledgeVault(opts Options, plan *Plan) {
	if opts.VaultPath == "" {
		return
	}
	plan.Servers = append(plan.Servers, filesystemServer("vault-fs", opts.VaultPath))
}

func addUserDocsVault(opts Options, plan *Plan) {
	if opts.UserDocsVaultPath == "" {
		return // "condicional": sem path configurado, sem server (aviso é UX de fase posterior).
	}
	plan.Servers = append(plan.Servers, filesystemServer("user-docs", opts.UserDocsVaultPath))
}

// filesystemServer monta um server @modelcontextprotocol/server-filesystem apontado a path.
func filesystemServer(name, path string) mcp.Server {
	return mcp.Server{
		Name:    name,
		Command: "npx",
		Args:    []string{"-y", "@modelcontextprotocol/server-filesystem", path},
	}
}

func addSecondBrain(plan *Plan) {
	plan.Globals = append(plan.Globals, GlobalStep{
		Key: "second_brain",
		Commands: []runner.Command{{
			Name: "npx",
			Args: []string{"skills", "add", "eugeniughelbur/obsidian-second-brain", "--skill", "obsidian-second-brain", "-a", "claude-code", "-g", "-y"},
		}},
	})
}

func addObsidianFormats(plan *Plan) {
	plan.Globals = append(plan.Globals, GlobalStep{
		Key: "obsidian_formats",
		Commands: []runner.Command{{
			Name: "npx",
			Args: []string{"skills", "add", "kepano/obsidian-skills", "--skill", "obsidian-markdown", "--skill", "json-canvas", "-a", "claude-code", "-g", "-y"},
		}},
	})
}

func addCodeGraph(plan *Plan) {
	plan.Globals = append(plan.Globals, GlobalStep{
		Key: "code_graph",
		Commands: []runner.Command{
			{Name: "uv", Args: []string{"tool", "install", "graphifyy"}},
			{Name: "graphify", Args: []string{"install", "--platform", "claude"}},
		},
	})
	plan.Commands = append(plan.Commands, runner.Command{Name: "graphify", Args: []string{"update", "."}})
	plan.Servers = append(plan.Servers, mcp.Server{Name: "graphify", Command: "graphify-mcp"})
}
