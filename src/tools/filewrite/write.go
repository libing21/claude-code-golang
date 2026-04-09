package filewrite

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"claude-code-running-go/src/tool"
	"claude-code-running-go/src/utils/permissions"
)

type Input struct {
	FilePath string `json:"file_path"`
	Content  string `json:"content"`
}

type WriteTool struct{}

func New() *WriteTool { return &WriteTool{} }

func (t *WriteTool) Name() string { return "Write" }

func (t *WriteTool) Prompt() string {
	return strings.TrimSpace(`- Creates or overwrites a file with the provided content
- Use this to create files instead of shell redirection`)
}

func (t *WriteTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type":"object",
  "additionalProperties":false,
  "properties":{
    "file_path":{"type":"string","description":"Path to the file to create or overwrite"},
    "content":{"type":"string","description":"Full file contents"}
  },
  "required":["file_path","content"]
}`)
}

func (t *WriteTool) IsReadOnly(_ any) bool        { return false }
func (t *WriteTool) IsConcurrencySafe(_ any) bool { return false }

func (t *WriteTool) ValidateInput(input any) error {
	switch v := input.(type) {
	case Input:
		if strings.TrimSpace(v.FilePath) == "" {
			return fmt.Errorf("file_path is required")
		}
	}
	return nil
}

func (t *WriteTool) CheckPermissions(_ context.Context, input any, _ tool.PermissionContext) (tool.PermissionDecision, any, error) {
	var p string
	switch v := input.(type) {
	case Input:
		p = v.FilePath
	case map[string]any:
		if s, ok := v["file_path"].(string); ok {
			p = s
		}
	}
	dec, abs := permissions.CheckPath(permissions.DefaultPathPolicy(), permissions.OpWrite, p)
	// If the path is allowed, defer to outer permission (mode/rules).
	if dec.Behavior == tool.PermissionBehaviorAllow {
		dec = tool.PermissionDecision{Behavior: tool.PermissionBehaviorPassthrough, Reason: "path allowed"}
	}
	if abs != "" {
		switch v := input.(type) {
		case Input:
			v.FilePath = abs
			return dec, v, nil
		case map[string]any:
			v["file_path"] = abs
			return dec, v, nil
		}
	}
	return dec, nil, nil
}

func (t *WriteTool) Call(_ context.Context, input any) (tool.ToolResult, error) {
	var in Input
	switch v := input.(type) {
	case Input:
		in = v
	case map[string]any:
		b, _ := json.Marshal(v)
		if err := json.Unmarshal(b, &in); err != nil {
			return tool.ToolResult{IsError: true, Content: "invalid input"}, err
		}
	default:
		return tool.ToolResult{IsError: true, Content: "invalid input type"}, fmt.Errorf("invalid input type %T", input)
	}
	p := filepath.Clean(in.FilePath)
	if !filepath.IsAbs(p) {
		cwd, _ := os.Getwd()
		p = filepath.Join(cwd, p)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return tool.ToolResult{IsError: true, Content: err.Error()}, nil
	}
	if err := os.WriteFile(p, []byte(in.Content), 0o644); err != nil {
		return tool.ToolResult{IsError: true, Content: err.Error()}, nil
	}
	return tool.ToolResult{Content: "File written successfully at: " + p}, nil
}
