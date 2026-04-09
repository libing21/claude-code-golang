package fileedit

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
	FilePath   string `json:"file_path"`
	OldString  string `json:"old_string"`
	NewString  string `json:"new_string"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

type EditTool struct{}

func New() *EditTool { return &EditTool{} }

func (t *EditTool) Name() string { return "Edit" }

func (t *EditTool) Prompt() string {
	return strings.TrimSpace(`- String-based file edit tool
- Replaces old_string with new_string in a target file
- Use replace_all to replace every occurrence`)
}

func (t *EditTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type":"object",
  "additionalProperties":false,
  "properties":{
    "file_path":{"type":"string","description":"Path to the file to modify"},
    "old_string":{"type":"string","description":"Text to replace"},
    "new_string":{"type":"string","description":"Replacement text"},
    "replace_all":{"type":"boolean","description":"Replace all occurrences"}
  },
  "required":["file_path","old_string","new_string"]
}`)
}

func (t *EditTool) IsReadOnly(_ any) bool        { return false }
func (t *EditTool) IsConcurrencySafe(_ any) bool { return false }

func (t *EditTool) ValidateInput(input any) error {
	switch v := input.(type) {
	case Input:
		if strings.TrimSpace(v.FilePath) == "" {
			return fmt.Errorf("file_path is required")
		}
		if v.OldString == "" {
			return fmt.Errorf("old_string is required")
		}
		if v.OldString == v.NewString {
			return fmt.Errorf("new_string must differ from old_string")
		}
	}
	return nil
}

func (t *EditTool) CheckPermissions(_ context.Context, input any, _ tool.PermissionContext) (tool.PermissionDecision, any, error) {
	var p string
	switch v := input.(type) {
	case Input:
		p = v.FilePath
	case map[string]any:
		if s, ok := v["file_path"].(string); ok {
			p = s
		}
	}
	dec, abs := permissions.CheckPath(permissions.DefaultPathPolicy(), permissions.OpEdit, p)
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

func (t *EditTool) Call(_ context.Context, input any) (tool.ToolResult, error) {
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
	raw, err := os.ReadFile(p)
	if err != nil {
		return tool.ToolResult{IsError: true, Content: err.Error()}, nil
	}
	contents := string(raw)
	if !strings.Contains(contents, in.OldString) {
		return tool.ToolResult{
			IsError: true,
			Content: "old_string not found in file",
		}, nil
	}

	var updated string
	if in.ReplaceAll {
		updated = strings.ReplaceAll(contents, in.OldString, in.NewString)
	} else {
		if strings.Count(contents, in.OldString) != 1 {
			return tool.ToolResult{
				IsError: true,
				Content: "old_string is not unique; use a more specific match or replace_all",
			}, nil
		}
		updated = strings.Replace(contents, in.OldString, in.NewString, 1)
	}

	if err := os.WriteFile(p, []byte(updated), 0o644); err != nil {
		return tool.ToolResult{IsError: true, Content: err.Error()}, nil
	}
	return tool.ToolResult{Content: "File edited successfully at: " + p}, nil
}
