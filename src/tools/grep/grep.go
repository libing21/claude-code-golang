package grep

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"claude-code-running-go/src/tool"
	"claude-code-running-go/src/utils/permissions"
)

type Input struct {
	Pattern    string `json:"pattern"`
	Path       string `json:"path,omitempty"`
	Glob       string `json:"glob,omitempty"`        // e.g. "*.ts"
	OutputMode string `json:"output_mode,omitempty"` // "files_with_matches" | "content"
	HeadLimit  int    `json:"head_limit,omitempty"`  // default 100
}

type GrepTool struct{}

func New() *GrepTool { return &GrepTool{} }

func (t *GrepTool) Name() string { return "Grep" }

func (t *GrepTool) Prompt() string {
	return strings.TrimSpace(`- Search tool built on Go regex scanning
- Supports searching within a directory tree with optional filename glob filter
- Output modes: "files_with_matches" (default) or "content"`)
}

func (t *GrepTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type":"object",
  "additionalProperties":false,
  "properties":{
    "pattern":{"type":"string","description":"Regex pattern"},
    "path":{"type":"string","description":"Directory root to search (optional)"},
    "glob":{"type":"string","description":"Filename glob filter, e.g. \"*.go\" (optional)"},
    "output_mode":{"type":"string","enum":["files_with_matches","content"],"description":"Output mode (optional)"},
    "head_limit":{"type":"integer","minimum":1,"description":"Max results (optional)"}
  },
  "required":["pattern"]
}`)
}

func (t *GrepTool) IsReadOnly(_ any) bool        { return true }
func (t *GrepTool) IsConcurrencySafe(_ any) bool { return true }

func (t *GrepTool) ValidateInput(input any) error {
	switch v := input.(type) {
	case Input:
		if strings.TrimSpace(v.Pattern) == "" {
			return fmt.Errorf("pattern is required")
		}
	}
	return nil
}

func (t *GrepTool) CheckPermissions(_ context.Context, input any, _ tool.PermissionContext) (tool.PermissionDecision, any, error) {
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
	dec, abs := permissions.CheckPath(permissions.DefaultPathPolicy(), permissions.OpGrep, p)
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

func (t *GrepTool) Call(ctx context.Context, input any) (tool.ToolResult, error) {
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

	re, err := regexp.Compile(in.Pattern)
	if err != nil {
		return tool.ToolResult{IsError: true, Content: err.Error()}, nil
	}

	root := in.Path
	if strings.TrimSpace(root) == "" {
		cwd, _ := os.Getwd()
		root = cwd
	}
	root, _ = filepath.Abs(root)

	mode := in.OutputMode
	if mode == "" {
		mode = "files_with_matches"
	}
	limit := in.HeadLimit
	if limit <= 0 {
		limit = 100
	}

	var bld strings.Builder
	count := 0

	matchFile := func(name string) bool {
		if in.Glob == "" {
			return true
		}
		ok, _ := filepath.Match(in.Glob, filepath.Base(name))
		return ok
	}

	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		if d.IsDir() {
			base := filepath.Base(path)
			if base == ".git" || base == "node_modules" || base == "dist" || base == "build" {
				return filepath.SkipDir
			}
			return nil
		}
		if !matchFile(path) {
			return nil
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 2*1024*1024)
		lineNo := 0
		fileMatched := false
		for sc.Scan() {
			lineNo++
			line := sc.Text()
			if re.MatchString(line) {
				if mode == "files_with_matches" {
					fileMatched = true
					break
				}
				rel, _ := filepath.Rel(root, path)
				fmt.Fprintf(&bld, "%s:%d:%s\n", rel, lineNo, line)
				count++
				if count >= limit {
					return io.EOF
				}
			}
		}
		if mode == "files_with_matches" && fileMatched {
			rel, _ := filepath.Rel(root, path)
			bld.WriteString(rel)
			bld.WriteString("\n")
			count++
			if count >= limit {
				return io.EOF
			}
		}
		return nil
	})
	if err != nil && err != io.EOF && err != context.Canceled {
		// Return errors only when it was cancellation or hard IO; otherwise ignore.
		if err == ctx.Err() {
			return tool.ToolResult{IsError: true, Content: "cancelled"}, err
		}
	}

	out := strings.TrimRight(bld.String(), "\n")
	if out == "" {
		out = "No matches found"
	}
	return tool.ToolResult{Content: out}, nil
}
