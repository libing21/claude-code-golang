package glob

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"claude-code-running-go/src/tool"
	"claude-code-running-go/src/utils/permissions"
	"github.com/bmatcuk/doublestar/v4"
)

type Input struct {
	Pattern string `json:"pattern"`
	Path    string `json:"path,omitempty"`
}

type GlobTool struct{}

func New() *GlobTool { return &GlobTool{} }

func (t *GlobTool) Name() string { return "Glob" }

func (t *GlobTool) Prompt() string {
	return strings.TrimSpace(`- Fast file pattern matching tool that works with any codebase size
- Supports glob patterns like "**/*.js" or "src/**/*.ts"
- Returns matching file paths sorted by modification time`)
}

func (t *GlobTool) InputSchema() json.RawMessage {
	// Minimal JSON Schema to keep things explicit & debuggable.
	return json.RawMessage(`{
  "type":"object",
  "additionalProperties":false,
  "properties":{
    "pattern":{"type":"string","description":"Glob pattern"},
    "path":{"type":"string","description":"Directory to search in (optional)"}
  },
  "required":["pattern"]
}`)
}

func (t *GlobTool) IsReadOnly(_ any) bool        { return true }
func (t *GlobTool) IsConcurrencySafe(_ any) bool { return true }

func (t *GlobTool) ValidateInput(input any) error {
	in, ok := input.(Input)
	if ok {
		if strings.TrimSpace(in.Pattern) == "" {
			return fmt.Errorf("pattern is required")
		}
		return nil
	}
	// When coming from generic JSON decode, it will be map[string]any; validate later.
	return nil
}

func (t *GlobTool) CheckPermissions(_ context.Context, input any, _ tool.PermissionContext) (tool.PermissionDecision, any, error) {
	var p string
	switch v := input.(type) {
	case Input:
		p = v.Path
	case map[string]any:
		if s, ok := v["path"].(string); ok {
			p = s
		}
	}
	if strings.TrimSpace(p) == "" {
		p = "."
	}
	dec, abs := permissions.CheckPath(permissions.DefaultPathPolicy(), permissions.OpGlob, p)
	if abs != "" {
		switch v := input.(type) {
		case Input:
			v.Path = abs
			return dec, v, nil
		case map[string]any:
			v["path"] = abs
			return dec, v, nil
		}
	}
	return dec, nil, nil
}

func (t *GlobTool) Call(ctx context.Context, input any) (tool.ToolResult, error) {
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

	base := in.Path
	if strings.TrimSpace(base) == "" {
		cwd, _ := os.Getwd()
		base = cwd
	}
	base, _ = filepath.Abs(base)

	// doublestar.Glob expects a filesystem path pattern. Anchor with base.
	pat := filepath.Join(base, filepath.FromSlash(in.Pattern))
	// Use DirFS at filesystem root so we can pass an absolute-ish slash path.
	// doublestar expects forward slashes on fs.FS paths.
	matches, err := doublestar.Glob(os.DirFS("/"), filepath.ToSlash(pat))
	if err != nil {
		return tool.ToolResult{IsError: true, Content: err.Error()}, nil
	}

	type item struct {
		path string
		mod  int64
	}
	items := make([]item, 0, len(matches))
	for _, m := range matches {
		select {
		case <-ctx.Done():
			return tool.ToolResult{IsError: true, Content: "cancelled"}, ctx.Err()
		default:
		}
		// Convert fs path back to OS path.
		osPath := filepath.FromSlash(m)
		// Avoid following broken entries.
		st, err := os.Stat(osPath)
		if err != nil {
			continue
		}
		// Skip directories.
		if st.IsDir() {
			continue
		}
		items = append(items, item{path: osPath, mod: st.ModTime().UnixNano()})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].mod > items[j].mod })

	var bld strings.Builder
	limit := 100
	if len(items) < limit {
		limit = len(items)
	}
	for i := 0; i < limit; i++ {
		rel := items[i].path
		if r, err := filepath.Rel(base, items[i].path); err == nil && !strings.HasPrefix(r, "..") {
			rel = r
		}
		bld.WriteString(rel)
		bld.WriteString("\n")
	}
	if len(items) > limit {
		bld.WriteString("(Results are truncated. Consider using a more specific path or pattern.)\n")
	}
	return tool.ToolResult{Content: strings.TrimRight(bld.String(), "\n")}, nil
}
