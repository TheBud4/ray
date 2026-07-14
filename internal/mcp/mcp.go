// Package mcp modela servidores MCP e faz o merge idempotente de .mcp.json.
package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// Server é uma entrada de mcpServers no .mcp.json.
type Server struct {
	Name    string
	Command string
	Args    []string
	Env     map[string]string
}

// serverJSON é a forma serializada de um Server (sem Name: a chave do mapa já é o nome).
type serverJSON struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// WriteServers mescla servers em <target>/.mcp.json: cada Server é definido por
// nome (substitui o de mesmo nome, preserva o resto do arquivo). dryRun imprime
// o resultado em out em vez de gravar.
func WriteServers(target string, servers []Server, dryRun bool, out io.Writer) error {
	path := filepath.Join(target, ".mcp.json")

	doc := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &doc); err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	mcpServers, ok := doc["mcpServers"].(map[string]any)
	if !ok {
		mcpServers = map[string]any{}
	}
	for _, s := range servers {
		mcpServers[s.Name] = serverJSON{Command: s.Command, Args: s.Args, Env: s.Env}
	}
	doc["mcpServers"] = mcpServers

	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	if dryRun {
		_, err := out.Write(data)
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
