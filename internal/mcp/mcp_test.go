package mcp

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func readMcpJSON(t *testing.T, target string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(target, ".mcp.json"))
	if err != nil {
		t.Fatalf("reading .mcp.json: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("parsing .mcp.json: %v", err)
	}
	return m
}

func TestWriteServersCreatesFromScratch(t *testing.T) {
	target := t.TempDir()
	servers := []Server{
		{Name: "headroom", Command: "headroom", Args: []string{"mcp"}},
	}
	if err := WriteServers(target, servers, false, nil); err != nil {
		t.Fatalf("WriteServers() error = %v", err)
	}

	m := readMcpJSON(t, target)
	got, ok := m["mcpServers"].(map[string]any)
	if !ok {
		t.Fatalf("mcpServers missing or wrong type: %#v", m["mcpServers"])
	}
	entry, ok := got["headroom"].(map[string]any)
	if !ok {
		t.Fatalf("headroom entry missing: %#v", got)
	}
	if entry["command"] != "headroom" {
		t.Fatalf("command = %v, want headroom", entry["command"])
	}
	if _, hasEnv := entry["env"]; hasEnv {
		t.Fatalf("env should be omitted when empty, got %#v", entry)
	}
}

func TestWriteServersPreservesExistingContentAndEntries(t *testing.T) {
	target := t.TempDir()
	existing := `{
  "otherTopLevelKey": "keep-me",
  "mcpServers": {
    "vault-fs": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/home/u/.ray/vault"]
    }
  }
}
`
	if err := os.WriteFile(filepath.Join(target, ".mcp.json"), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}

	servers := []Server{
		{Name: "headroom", Command: "headroom", Args: []string{"mcp"}},
	}
	if err := WriteServers(target, servers, false, nil); err != nil {
		t.Fatalf("WriteServers() error = %v", err)
	}

	m := readMcpJSON(t, target)
	if m["otherTopLevelKey"] != "keep-me" {
		t.Fatalf("otherTopLevelKey lost: %#v", m)
	}
	mcpServers := m["mcpServers"].(map[string]any)
	if _, ok := mcpServers["vault-fs"]; !ok {
		t.Fatalf("existing server vault-fs lost: %#v", mcpServers)
	}
	if _, ok := mcpServers["headroom"]; !ok {
		t.Fatalf("new server headroom not added: %#v", mcpServers)
	}
}

func TestWriteServersReplacesSameNameWithoutDuplicating(t *testing.T) {
	target := t.TempDir()

	first := []Server{{Name: "headroom", Command: "old-command"}}
	if err := WriteServers(target, first, false, nil); err != nil {
		t.Fatal(err)
	}
	second := []Server{{Name: "headroom", Command: "headroom", Args: []string{"mcp"}}}
	if err := WriteServers(target, second, false, nil); err != nil {
		t.Fatal(err)
	}

	m := readMcpJSON(t, target)
	mcpServers := m["mcpServers"].(map[string]any)
	if len(mcpServers) != 1 {
		t.Fatalf("mcpServers = %#v, want exactly one entry", mcpServers)
	}
	entry := mcpServers["headroom"].(map[string]any)
	if entry["command"] != "headroom" {
		t.Fatalf("command = %v, want updated value headroom", entry["command"])
	}
}

func TestWriteServersIsIdempotent(t *testing.T) {
	target := t.TempDir()
	servers := []Server{
		{Name: "headroom", Command: "headroom", Args: []string{"mcp"}},
		{Name: "vault-fs", Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-filesystem", "/vault"}},
	}
	if err := WriteServers(target, servers, false, nil); err != nil {
		t.Fatal(err)
	}
	first, err := os.ReadFile(filepath.Join(target, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteServers(target, servers, false, nil); err != nil {
		t.Fatal(err)
	}
	second, err := os.ReadFile(filepath.Join(target, ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("2nd application changed the file:\n--- 1st ---\n%s\n--- 2nd ---\n%s", first, second)
	}
	if !bytes.HasSuffix(second, []byte("\n")) {
		t.Fatalf("file does not end with newline: %q", second)
	}
}

func TestWriteServersDryRunDoesNotWrite(t *testing.T) {
	target := t.TempDir()
	var out bytes.Buffer
	servers := []Server{{Name: "headroom", Command: "headroom", Args: []string{"mcp"}}}

	if err := WriteServers(target, servers, true, &out); err != nil {
		t.Fatalf("WriteServers() error = %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, ".mcp.json")); !os.IsNotExist(err) {
		t.Fatalf(".mcp.json should not exist after dry-run, stat err = %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("dry-run should print the resulting JSON to out")
	}
}
