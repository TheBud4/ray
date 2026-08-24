package installer

import (
	"reflect"
	"slices"
	"testing"

	"github.com/TheBud4/ray/internal/mcp"
	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/runner"
)

// Componentes (`p.Components`) nunca passam por installer.Plan — são cópia
// local pura, resolvida direto em internal/initai e internal/update.

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
			Headroom: true, Brain: true, CodeGraph: true,
		},
		Components: []profile.Component{
			{Name: "prompt-engineer", Dest: ".claude/skills"},
			{Name: "code-reviewer", Dest: ".claude/agents"},
		},
	}
	opts := Options{BrainPath: "/home/u/www/MegaBrain"}

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
		"headroom":   {"uv tool install headroom-ai[mcp]"},
		"code_graph": {"uv tool install graphifyy", "graphify install --platform claude"},
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
		{Name: "brain", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-filesystem", "/home/u/www/MegaBrain"}},
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
			{Name: "s", Dest: ".claude/skills"},
		},
	}
	plan, err := Resolve(p, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Commands) != 0 {
		t.Fatalf("Commands = %v, want none: components are copied directly by internal/initai and internal/update, not through installer.Plan", cmdStrings(plan.Commands))
	}
	if len(plan.Globals) != 0 || len(plan.Servers) != 0 {
		t.Fatalf("want no globals/servers, got %d/%d", len(plan.Globals), len(plan.Servers))
	}
}

func TestResolveBrainUnset(t *testing.T) {
	p := &profile.Profile{
		Name:         "brain",
		Integrations: profile.Integrations{Brain: true},
	}
	plan, err := Resolve(p, Options{BrainPath: ""})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Servers) != 0 {
		t.Fatalf("Servers = %#v, want none", plan.Servers)
	}
}
