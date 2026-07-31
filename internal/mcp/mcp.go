// Package mcp modela servidores MCP e faz o merge idempotente de .mcp.json.
package mcp

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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

// fileName é o arquivo de configuração de MCP na raiz do projeto. Constante
// porque ReadServers e WriteServers têm de concordar sobre ele.
const fileName = ".mcp.json"

// ReadServers lê os servidores de <target>/.mcp.json, ordenados por nome para
// a saída ser estável. Arquivo ausente devolve lista vazia sem erro — projeto
// sem MCP é caso normal, não falha.
func ReadServers(target string) ([]Server, error) {
	data, err := os.ReadFile(filepath.Join(target, fileName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var doc struct {
		Servers map[string]serverJSON `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", fileName, err)
	}
	out := make([]Server, 0, len(doc.Servers))
	for name, s := range doc.Servers {
		out = append(out, Server{Name: name, Command: s.Command, Args: s.Args, Env: s.Env})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// WriteServers mescla servers em <target>/.mcp.json: cada Server é definido por
// nome (substitui o de mesmo nome, preserva o resto do arquivo). dryRun imprime
// o resultado em out em vez de gravar.
func WriteServers(target string, servers []Server, dryRun bool, out io.Writer) error {
	path := filepath.Join(target, fileName)

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
