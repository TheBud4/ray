package installer

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/TheBud4/ray/internal/mcp"
	"github.com/TheBud4/ray/internal/profile"
	"github.com/TheBud4/ray/internal/runner"
)

func TestComponentCommand(t *testing.T) {
	cases := []struct {
		name    string
		comp    profile.Component
		opts    Options
		want    string
		wantErr string
	}{
		{
			name: "skill local",
			comp: profile.Component{Via: profile.ViaSkills, Skill: "prompt-engineer", Source: "jeffallan/claude-skills"},
			want: "npx skills add jeffallan/claude-skills --skill prompt-engineer -a claude-code -y",
		},
		{
			name: "skill global",
			comp: profile.Component{Via: profile.ViaSkills, Skill: "prompt-engineer", Source: "jeffallan/claude-skills"},
			opts: Options{Global: true},
			want: "npx skills add jeffallan/claude-skills --skill prompt-engineer -a claude-code -y -g",
		},
		{
			name: "aitmpl agent",
			comp: profile.Component{Via: profile.ViaAitmpl, Type: profile.TypeAgent, Ref: "development-tools/code-reviewer"},
			want: "npx claude-code-templates@latest --agent=development-tools/code-reviewer --yes",
		},
		{
			// aitmpl (agents/commands) não tem noção de --global (I1 design
			// §3.3): sempre project-local, mesmo com opts.Global.
			name: "aitmpl agent ignores Global",
			comp: profile.Component{Via: profile.ViaAitmpl, Type: profile.TypeAgent, Ref: "development-tools/code-reviewer"},
			opts: Options{Global: true},
			want: "npx claude-code-templates@latest --agent=development-tools/code-reviewer --yes",
		},
		{
			name: "aitmpl command",
			comp: profile.Component{Via: profile.ViaAitmpl, Type: profile.TypeCommand, Ref: "development-tools/document"},
			want: "npx claude-code-templates@latest --command=development-tools/document --yes",
		},
		{
			name: "aitmpl mcp",
			comp: profile.Component{Via: profile.ViaAitmpl, Type: profile.TypeMCP, Ref: "some/server"},
			want: "npx claude-code-templates@latest --mcp=some/server --yes",
		},
		{
			name:    "unknown via",
			comp:    profile.Component{Via: "bogus"},
			wantErr: "unknown via",
		},
		{
			name:    "unknown aitmpl type",
			comp:    profile.Component{Via: profile.ViaAitmpl, Type: "tool", Ref: "x"},
			wantErr: "unknown aitmpl type",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := componentCommand(tc.comp, tc.opts)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("componentCommand() = nil error, want %q", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("componentCommand() error = %v", err)
			}
			if got.String() != tc.want {
				t.Fatalf("got %q, want %q", got.String(), tc.want)
			}
			if got.Dir != "" {
				t.Fatalf("Dir = %q, want empty (set later by initai)", got.Dir)
			}
		})
	}
}

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

	wantCmds := []string{
		"npx skills add jeffallan/claude-skills --skill prompt-engineer -a claude-code -y -g",
		"npx claude-code-templates@latest --agent=development-tools/code-reviewer --yes",
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
	if len(plan.Commands) != 1 {
		t.Fatalf("Commands = %v, want exactly the one component", cmdStrings(plan.Commands))
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

func TestResolveUnknownViaPropagates(t *testing.T) {
	p := &profile.Profile{
		Name:       "bad",
		Components: []profile.Component{{Via: "bogus"}},
	}
	if _, err := Resolve(p, Options{}); err == nil {
		t.Fatal("Resolve() = nil error, want unknown via")
	}
}
