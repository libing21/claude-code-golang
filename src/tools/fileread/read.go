package fileread

import (
	"bufio"
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
	Offset   int    `json:"offset,omitempty"` // 1-based line offset
	Limit    int    `json:"limit,omitempty"`
}

type ReadTool struct{}

func New() *ReadTool { return &ReadTool{} }

func (t *ReadTool) Name() string { return "Read" }

func (t *ReadTool) Prompt() string {
	return strings.TrimSpace(`- Reads a file from the local filesystem.
- Supports optional line offset/limit for large files.
- Use this instead of cat/head/tail when you need file content.`)
}

func (t *ReadTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type":"object",
  "additionalProperties":false,
  "properties":{
    "file_path":{"type":"string","description":"Absolute or relative path to the file"},
    "offset":{"type":"integer","minimum":1,"description":"1-based line offset (optional)"},
    "limit":{"type":"integer","minimum":1,"description":"Number of lines to read (optional)"}
  },
  "required":["file_path"]
}`)
}

func (t *ReadTool) IsReadOnly(_ any) bool        { return true }
func (t *ReadTool) IsConcurrencySafe(_ any) bool { return true }

func (t *ReadTool) ValidateInput(input any) error {
	// Minimal validation, detailed fs errors occur in Call.
	switch v := input.(type) {
	case Input:
		if strings.TrimSpace(v.FilePath) == "" {
			return fmt.Errorf("file_path is required")
		}
	}
	return nil
}

func (t *ReadTool) CheckPermissions(_ context.Context, input any, _ tool.PermissionContext) (tool.PermissionDecision, any, error) {
	// Apply filesystem safety checks (TS parity).
	var p string
	switch v := input.(type) {
	case Input:
		p = v.FilePath
	case map[string]any:
		if s, ok := v["file_path"].(string); ok {
			p = s
		}
	}
	dec, abs := permissions.CheckPath(permissions.DefaultPathPolicy(), permissions.OpRead, p)
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

func (t *ReadTool) Call(ctx context.Context, input any) (tool.ToolResult, error) {
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
	f, err := os.Open(p)
	if err != nil {
		return tool.ToolResult{IsError: true, Content: err.Error()}, nil
	}
	defer f.Close()

	offset := in.Offset
	limit := in.Limit
	if offset <= 0 {
		offset = 1
	}
	if limit < 0 {
		limit = 0
	}

	var bld strings.Builder
	sc := bufio.NewScanner(f)
	// Increase buffer for long lines.
	sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
	lineNo := 0
	wrote := 0
	for sc.Scan() {
		select {
		case <-ctx.Done():
			return tool.ToolResult{IsError: true, Content: "cancelled"}, ctx.Err()
		default:
		}
		lineNo++
		if lineNo < offset {
			continue
		}
		if limit > 0 && wrote >= limit {
			break
		}
		wrote++
		fmt.Fprintf(&bld, "%d→%s\n", lineNo, sc.Text())
	}
	if err := sc.Err(); err != nil {
		return tool.ToolResult{IsError: true, Content: err.Error()}, nil
	}
	if wrote == 0 {
		return tool.ToolResult{Content: "The file has 0 lines."}, nil
	}
	return tool.ToolResult{Content: strings.TrimRight(bld.String(), "\n")}, nil
}
