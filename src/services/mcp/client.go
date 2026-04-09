package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// MCP methods used here follow the current MCP conventions:
// - tools/list
// - tools/call
// We keep params/result loosely typed for compatibility.

type toolListResult struct {
	Tools []ToolDef `json:"tools"`
}

type toolCallParams struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

type toolCallResult struct {
	Content any `json:"content"` // string or block array
}

const maxMcpDescriptionLength = 2048

func IsMcpInstructionsDeltaEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("CLAUDE_GO_MCP_INSTRUCTIONS_DELTA")))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

type initializeParams struct {
	ProtocolVersion string         `json:"protocolVersion"`
	Capabilities    map[string]any `json:"capabilities"`
	ClientInfo      map[string]any `json:"clientInfo"`
}

type initializeResult struct {
	ProtocolVersion string         `json:"protocolVersion,omitempty"`
	Capabilities    map[string]any `json:"capabilities,omitempty"`
	ServerInfo      map[string]any `json:"serverInfo,omitempty"`
	Instructions    string         `json:"instructions,omitempty"`
}

func Initialize(ctx context.Context, c *Client) error {
	var res initializeResult
	params := initializeParams{
		ProtocolVersion: "2024-11-05",
		Capabilities:    map[string]any{},
		ClientInfo: map[string]any{
			"name":    "claude-code-running-go",
			"version": "0.1",
		},
	}
	if err := c.Call(ctx, "initialize", params, &res); err != nil {
		return err
	}
	// Follow-up notification per MCP convention.
	_ = c.Notify(ctx, "notifications/initialized", map[string]any{})

	if res.Instructions != "" {
		raw := res.Instructions
		if len(raw) > maxMcpDescriptionLength {
			raw = raw[:maxMcpDescriptionLength] + "… [truncated]"
		}
		c.setInstructions(raw)
	}
	return nil
}

func ListTools(ctx context.Context, c *Client) ([]ToolDef, error) {
	var res toolListResult
	if err := c.Call(ctx, "tools/list", nil, &res); err != nil {
		return nil, err
	}
	return res.Tools, nil
}

func CallTool(ctx context.Context, c *Client, name string, args map[string]any) (any, error) {
	var res toolCallResult
	if err := c.Call(ctx, "tools/call", toolCallParams{Name: name, Arguments: args}, &res); err != nil {
		return nil, err
	}
	// Normalize to string or []blocks; leave as-is for tool_result.
	switch res.Content.(type) {
	case string, []any, []map[string]any, []json.RawMessage:
		return res.Content, nil
	default:
		// If unknown, stringify for safety.
		b, _ := json.Marshal(res.Content)
		return fmt.Sprintf("%s", string(b)), nil
	}
}
