package mcp

import "encoding/json"

// Minimal MCP types needed for tool list/call.
// We intentionally keep these un-opinionated to ease 1:1 migration.

type ServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

// File format mirrors common .mcp.json layouts: { "mcpServers": { "name": { ... } } }
type ConfigFile struct {
	McpServers map[string]ServerConfig `json:"mcpServers"`
}

type ToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	InputSchema json.RawMessage `json:"inputSchema,omitempty"`
}

