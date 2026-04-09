package toolsearch

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"claude-code-running-go/src/tool"
)

// ToolSearch is a minimal Go analog of TS ToolSearchTool.
// It returns a list of "deferred tool" names matching the query.
//
// NOTE: This Go port does not implement SDK-level deferred tool loading; ToolSearch
// exists for runtime parity and can be used by the model to discover tools by name.

type Input struct {
	Query      string `json:"query"`
	MaxResults int    `json:"max_results,omitempty"`
}

type ToolSearchTool struct {
	listDeferred func() []tool.Tool
}

func New(listDeferred func() []tool.Tool) *ToolSearchTool {
	return &ToolSearchTool{listDeferred: listDeferred}
}

func (t *ToolSearchTool) Name() string { return "ToolSearch" }

func (t *ToolSearchTool) Prompt() string {
	return "Search for available tool names. Returns matching tool names that can be used in tool calls."
}

func (t *ToolSearchTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type":"object",
  "properties":{
    "query":{"type":"string"},
    "max_results":{"type":"integer","minimum":1,"maximum":50}
  },
  "required":["query"],
  "additionalProperties":false
}`)
}

func (t *ToolSearchTool) IsReadOnly(_ any) bool        { return true }
func (t *ToolSearchTool) IsConcurrencySafe(_ any) bool { return true }
func (t *ToolSearchTool) ValidateInput(_ any) error    { return nil }

func (t *ToolSearchTool) CheckPermissions(_ context.Context, _ any, _ tool.PermissionContext) (tool.PermissionDecision, any, error) {
	return tool.PermissionDecision{Behavior: tool.PermissionBehaviorAllow}, nil, nil
}

func (t *ToolSearchTool) Call(_ context.Context, input any) (tool.ToolResult, error) {
	in := Input{MaxResults: 10}
	b, _ := json.Marshal(input)
	_ = json.Unmarshal(b, &in)
	q := strings.ToLower(strings.TrimSpace(in.Query))
	if q == "" {
		return tool.ToolResult{IsError: true, Content: "query is required"}, nil
	}
	if in.MaxResults <= 0 {
		in.MaxResults = 10
	}

	deferred := []tool.Tool{}
	if t.listDeferred != nil {
		deferred = t.listDeferred()
	}
	sort.Slice(deferred, func(i, j int) bool { return deferred[i].Name() < deferred[j].Name() })

	matches := make([]string, 0, in.MaxResults)
	for _, dt := range deferred {
		name := dt.Name()
		if strings.Contains(strings.ToLower(name), q) {
			matches = append(matches, name)
			if len(matches) >= in.MaxResults {
				break
			}
		}
	}
	if len(matches) == 0 {
		return tool.ToolResult{Content: "No matching tools."}, nil
	}
	// TS returns tool_reference blocks when the model supports it.
	blocks := make([]any, 0, len(matches))
	for _, name := range matches {
		blocks = append(blocks, map[string]any{
			"type":      "tool_reference",
			"tool_name": name,
		})
	}
	return tool.ToolResult{Content: blocks}, nil
}
