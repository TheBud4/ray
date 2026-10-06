package economy

import (
	"reflect"
	"testing"

	"github.com/TheBud4/ray/internal/mcp"
	"github.com/TheBud4/ray/internal/runner"
)

func TestHeadroomFields(t *testing.T) {
	m := Headroom()
	if m.Name != "headroom" {
		t.Errorf("Name = %q, want %q", m.Name, "headroom")
	}
	if m.Kind != "mcp" {
		t.Errorf("Kind = %q, want %q", m.Kind, "mcp")
	}
	wantInstall := []runner.Command{{Name: "uv", Args: []string{"tool", "install", "headroom-ai[mcp]"}}}
	if !reflect.DeepEqual(m.Install, wantInstall) {
		t.Errorf("Install = %#v, want %#v", m.Install, wantInstall)
	}
	if m.Commands != nil {
		t.Errorf("Commands = %#v, want nil (headroom has no per-project command)", m.Commands)
	}
	wantServer := &mcp.Server{Name: "headroom", Command: "headroom", Args: []string{"mcp"}}
	if !reflect.DeepEqual(m.Server, wantServer) {
		t.Errorf("Server = %#v, want %#v", m.Server, wantServer)
	}
	if m.MetricKey == "" {
		t.Error("MetricKey is empty")
	}
}

func TestCodeGraphFields(t *testing.T) {
	m := CodeGraph()
	if m.Name != "code_graph" {
		t.Errorf("Name = %q, want %q", m.Name, "code_graph")
	}
	if m.Kind != "mcp" {
		t.Errorf("Kind = %q, want %q", m.Kind, "mcp")
	}
	wantInstall := []runner.Command{
		{Name: "uv", Args: []string{"tool", "install", "graphifyy"}},
		{Name: "graphify", Args: []string{"install", "--platform", "claude"}},
	}
	if !reflect.DeepEqual(m.Install, wantInstall) {
		t.Errorf("Install = %#v, want %#v", m.Install, wantInstall)
	}
	wantCommands := []runner.Command{{Name: "graphify", Args: []string{"update", "."}}}
	if !reflect.DeepEqual(m.Commands, wantCommands) {
		t.Errorf("Commands = %#v, want %#v", m.Commands, wantCommands)
	}
	wantServer := &mcp.Server{Name: "graphify", Command: "graphify-mcp"}
	if !reflect.DeepEqual(m.Server, wantServer) {
		t.Errorf("Server = %#v, want %#v", m.Server, wantServer)
	}
	if m.MetricKey == "" {
		t.Error("MetricKey is empty")
	}
}

func TestHandoffFields(t *testing.T) {
	m := Handoff()
	if m.Name != "handoff" {
		t.Errorf("Name = %q, want %q", m.Name, "handoff")
	}
	if m.Kind != "hook" {
		t.Errorf("Kind = %q, want %q", m.Kind, "hook")
	}
	if m.Install != nil || m.Commands != nil || m.Server != nil {
		t.Errorf("Install/Commands/Server = %#v/%#v/%#v, want all nil (handoff is scaffold-only)", m.Install, m.Commands, m.Server)
	}
	if m.MetricKey == "" {
		t.Error("MetricKey is empty")
	}
}
