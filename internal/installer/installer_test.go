package installer

import (
	"reflect"
	"slices"
	"testing"

	"github.com/TheBud4/ray/internal/mcp"
	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/runner"
)

// componentCommand/aitmplFlag moved to internal/acquire.CliAcquirer in I2
// (component installs are no longer part of installer.Plan.Commands — see
// internal/acquire/acquire_test.go for their coverage).

func cmdStrings(cs []runner.Command) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.String()
	}
	return out
}

func TestResolveAllIntegrations(t *testing.T) {
	p := &profile.Profile{
		Name: "go",
		Integrations: profile.Integrations{
			Headroom: true, KnowledgeVault: true, SecondBrain: true,
			ObsidianFormats: true, CodeGraph: true, UserDocsVault: true,
		},
		Components: []profile.Component{
			{Via: profile.ViaSkills, Skill: "prompt-engineer", Source: "jeffallan/claude-skills"},
			{Via: profile.ViaAitmpl, Type: profile.TypeAgent, Ref: "development-tools/code-reviewer"},
		},
	}
	opts := Options{Global: true, VaultPath: "/home/u/.ray/vault", UserDocsVaultPath: "/home/u/Docs"}

	plan, err := Resolve(p, opts)
	if err != nil {
		t.Fatal(err)
	}

	// Componentes (`p.Components`) não geram mais plan.Commands — só a
	// integração code_graph, que roda "graphify update ." como comando de
	// projeto independente de conteúdo (I2).
	wantCmds := []string{
		"graphify update .",
	}
	if got := cmdStrings(plan.Commands); !slices.Equal(got, wantCmds) {
		t.Fatalf("Commands = %v, want %v", got, wantCmds)
	}

	wantGlobals := map[string][]string{
		"headroom":         {"uv tool install headroom-ai[mcp]"},
		"second_brain":     {"npx skills add eugeniughelbur/obsidian-second-brain --skill obsidian-second-brain -a claude-code -g -y"},
		"obsidian_formats": {"npx skills add kepano/obsidian-skills --skill obsidian-markdown --skill json-canvas -a claude-code -g -y"},
		"code_graph":       {"uv tool install graphifyy", "graphify install --platform claude"},
	}
	if len(plan.Globals) != len(wantGlobals) {
		t.Fatalf("len(Globals) = %d, want %d", len(plan.Globals), len(wantGlobals))
	}
	for _, g := range plan.Globals {
		want, ok := wantGlobals[g.Key]
		if !ok {
			t.Fatalf("unexpected global key %q", g.Key)
		}
		if got := cmdStrings(g.Commands); !slices.Equal(got, want) {
			t.Fatalf("global %q Commands = %v, want %v", g.Key, got, want)
		}
	}

	wantServers := []mcp.Server{
		{Name: "headroom", Command: "headroom", Args: []string{"mcp"}},
		{Name: "vault-fs", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-filesystem", "/home/u/.ray/vault"}},
		{Name: "user-docs", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-filesystem", "/home/u/Docs"}},
		{Name: "graphify", Command: "graphify-mcp"},
	}
	if !reflect.DeepEqual(plan.Servers, wantServers) {
		t.Fatalf("Servers = %#v, want %#v", plan.Servers, wantServers)
	}
}

func TestResolveNoIntegrations(t *testing.T) {
	p := &profile.Profile{
		Name: "bare",
		Components: []profile.Component{
			{Via: profile.ViaSkills, Skill: "s", Source: "o/r"},
		},
	}
	plan, err := Resolve(p, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Commands) != 0 {
		t.Fatalf("Commands = %v, want none: components are acquired via internal/acquire, not installer.Plan", cmdStrings(plan.Commands))
	}
	if len(plan.Globals) != 0 || len(plan.Servers) != 0 {
		t.Fatalf("want no globals/servers, got %d/%d", len(plan.Globals), len(plan.Servers))
	}
}

func TestResolveUserDocsVaultUnset(t *testing.T) {
	p := &profile.Profile{
		Name:         "docs",
		Integrations: profile.Integrations{UserDocsVault: true},
	}
	plan, err := Resolve(p, Options{UserDocsVaultPath: ""})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Servers) != 0 {
		t.Fatalf("Servers = %#v, want none", plan.Servers)
	}
}
