package mcptool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"claude-code-running-go/src/services/mcp"
	"claude-code-running-go/src/tool"
)

// Tool is an adapter that exposes an MCP server tool as a local tool.Tool.
type Tool struct {
	serverName string
	client     *mcp.Client
	def        mcp.ToolDef
	toolName   string
}

func New(serverName string, client *mcp.Client, def mcp.ToolDef, exposedName string) *Tool {
	return &Tool{
		serverName: serverName,
		client:     client,
		def:        def,
		toolName:   exposedName,
	}
}

func (t *Tool) Name() string { return t.toolName }

// IsMCPTool allows runtime identification for deferred tools / deltas.
func (t *Tool) IsMCPTool() bool { return true }

func (t *Tool) MCPServerName() string { return t.serverName }

func (t *Tool) Prompt() string {
	desc := strings.TrimSpace(t.def.Description)
	if desc == "" {
		desc = "MCP tool from server " + t.serverName
	}
	return desc
}

func (t *Tool) InputSchema() json.RawMessage {
	if len(t.def.InputSchema) > 0 {
		return t.def.InputSchema
	}
	return json.RawMessage(`{"type":"object","additionalProperties":true}`)
}

func (t *Tool) IsReadOnly(_ any) bool        { return false }
func (t *Tool) IsConcurrencySafe(_ any) bool { return false }

func (t *Tool) ValidateInput(_ any) error { return nil }

func (t *Tool) CheckPermissions(_ context.Context, _ any, _ tool.PermissionContext) (tool.PermissionDecision, any, error) {
	// Treat as potentially dangerous: defer to outer policy (mode/rules).
	return tool.PermissionDecision{Behavior: tool.PermissionBehaviorPassthrough}, nil, nil
}

func (t *Tool) Call(ctx context.Context, input any) (tool.ToolResult, error) {
	args := map[string]any{}
	switch v := input.(type) {
	case map[string]any:
		args = v
	default:
		b, _ := json.Marshal(v)
		_ = json.Unmarshal(b, &args)
	}
	out, err := mcp.CallTool(ctx, t.client, t.def.Name, args)
	if err != nil {
		return tool.ToolResult{IsError: true, Content: err.Error()}, nil
	}

	// If it's blocks, return JSON for now; tool_result layer can pass blocks as-is.
	switch vv := out.(type) {
	case string:
		return tool.ToolResult{Content: vv}, nil
	default:
		b, _ := json.Marshal(vv)
		return tool.ToolResult{Content: fmt.Sprintf("%s", string(b))}, nil
	}
}
