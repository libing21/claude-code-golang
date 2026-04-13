package memorywrite

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"claude-code-running-go/src/memdir"
	"claude-code-running-go/src/tool"
)

// MemoryWrite is a guarded helper tool for writing long-term memories.
// It enforces the expected file format and updates MEMORY.md index automatically.
type Input struct {
	Scope          string `json:"scope"` // "private" | "team"
	Title          string `json:"title"`
	Description    string `json:"description"`
	Type           string `json:"type"`    // "user|feedback|project|reference"
	Content        string `json:"content"` // markdown body
	Filename       string `json:"filename,omitempty"`
	AllowSensitive bool   `json:"allow_sensitive,omitempty"` // only for private scope
}

type Tool struct{}

func New() *Tool { return &Tool{} }

func (t *Tool) Name() string { return "MemoryWrite" }

func (t *Tool) Prompt() string {
	return "Write a memory topic file and update MEMORY.md index (private or team)."
}

func (t *Tool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type":"object",
  "additionalProperties":false,
  "properties":{
    "scope":{"type":"string","enum":["private","team"]},
    "title":{"type":"string"},
    "description":{"type":"string"},
    "type":{"type":"string","description":"user|feedback|project|reference"},
    "content":{"type":"string"},
    "filename":{"type":"string"},
    "allow_sensitive":{"type":"boolean"}
  },
  "required":["scope","title","description","type","content"]
}`)
}

func (t *Tool) IsReadOnly(_ any) bool        { return false }
func (t *Tool) IsConcurrencySafe(_ any) bool { return false }

func (t *Tool) ValidateInput(input any) error {
	var in Input
	b, _ := json.Marshal(input)
	if err := json.Unmarshal(b, &in); err != nil {
		return err
	}
	if strings.TrimSpace(in.Scope) == "" {
		return fmt.Errorf("scope is required")
	}
	if strings.TrimSpace(in.Title) == "" {
		return fmt.Errorf("title is required")
	}
	if strings.TrimSpace(in.Description) == "" {
		return fmt.Errorf("description is required")
	}
	if strings.TrimSpace(in.Type) == "" {
		return fmt.Errorf("type is required")
	}
	if strings.TrimSpace(in.Content) == "" {
		return fmt.Errorf("content is required")
	}
	return nil
}

func (t *Tool) CheckPermissions(_ context.Context, _ any, _ tool.PermissionContext) (tool.PermissionDecision, any, error) {
	// Potentially writes to disk; defer to outer permission rules.
	return tool.PermissionDecision{Behavior: tool.PermissionBehaviorPassthrough}, nil, nil
}

func (t *Tool) Call(ctx context.Context, input any) (tool.ToolResult, error) {
	_ = ctx
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

	scope := strings.ToLower(strings.TrimSpace(in.Scope))
	if scope != "private" && scope != "team" {
		return tool.ToolResult{IsError: true, Content: "invalid scope"}, nil
	}
	if scope == "team" && containsSensitive(in.Title+"\n"+in.Description+"\n"+in.Content) {
		return tool.ToolResult{IsError: true, Content: "refusing to write sensitive content to team memory"}, nil
	}
	if scope == "private" && !in.AllowSensitive && containsSensitive(in.Title+"\n"+in.Description+"\n"+in.Content) {
		return tool.ToolResult{IsError: true, Content: "potential sensitive content detected; set allow_sensitive=true to force private write"}, nil
	}

	cwd, _ := os.Getwd()
	baseDir := memdir.GetAutoMemPath(cwd)
	if scope == "team" {
		baseDir = memdir.GetTeamMemPath(cwd)
	}
	_ = os.MkdirAll(baseDir, 0o755)

	filename := strings.TrimSpace(in.Filename)
	if filename == "" {
		filename = slugify(in.Title) + "-" + time.Now().Format("20060102-150405") + ".md"
	}
	if !strings.HasSuffix(strings.ToLower(filename), ".md") {
		filename += ".md"
	}
	filename = filepath.Base(filename)
	topicPath := filepath.Join(baseDir, filename)

	body := strings.TrimSpace(in.Content) + "\n"
	doc := strings.Join([]string{
		"---",
		`name: "` + escapeYAML(in.Title) + `"`,
		`description: "` + escapeYAML(in.Description) + `"`,
		`type: "` + escapeYAML(in.Type) + `"`,
		"---",
		"",
		body,
	}, "\n")
	if err := os.WriteFile(topicPath, []byte(doc), 0o644); err != nil {
		return tool.ToolResult{IsError: true, Content: "write failed: " + err.Error()}, nil
	}

	indexPath := filepath.Join(baseDir, memdir.ENTRYPOINT_NAME)
	if err := upsertIndexLine(indexPath, in.Title, filename, in.Description); err != nil {
		return tool.ToolResult{IsError: true, Content: "index update failed: " + err.Error()}, nil
	}
	return tool.ToolResult{
		Content: fmt.Sprintf("Memory saved.\n- scope: %s\n- topic: %s\n- index: %s", scope, topicPath, indexPath),
	}, nil
}

func escapeYAML(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return "memory"
	}
	var b strings.Builder
	lastDash := false
	for i := 0; i < len(s); i++ {
		ch := s[i]
		ok := (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')
		if ok {
			b.WriteByte(ch)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		return "memory"
	}
	return out
}

func upsertIndexLine(indexPath, title, filename, desc string) error {
	line := "- [" + strings.TrimSpace(title) + "](" + filename + ") — " + strings.TrimSpace(desc)
	existing, _ := os.ReadFile(indexPath)
	lines := []string{}
	if strings.TrimSpace(string(existing)) != "" {
		lines = strings.Split(strings.TrimRight(string(existing), "\n"), "\n")
	}
	found := false
	for i := range lines {
		if strings.Contains(lines[i], "]("+filename+")") {
			lines[i] = line
			found = true
		}
	}
	if !found {
		lines = append(lines, line)
	}
	content := strings.Join(lines, "\n") + "\n"
	return os.WriteFile(indexPath, []byte(content), 0o644)
}

func containsSensitive(s string) bool {
	s = strings.ToLower(s)
	for _, sub := range []string{
		"anthropic_api_key",
		"api_key",
		"apikey",
		"auth_token",
		"authorization:",
		"bearer ",
		"password",
		"secret",
		"sk-",
		"-----begin",
	} {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

