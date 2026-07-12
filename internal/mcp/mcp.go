// Package mcp modela servidores MCP e (na Fase 4) faz o merge idempotente de .mcp.json.
package mcp

// Server é uma entrada de mcpServers no .mcp.json.
// O writer que persiste isso chega na Fase 4; aqui é só o dado.
type Server struct {
	Name    string
	Command string
	Args    []string
	Env     map[string]string
}
