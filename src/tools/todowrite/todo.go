package todowrite

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"claude-code-running-go/src/tool"
)

type TodoItem struct {
	ID       string `json:"id"`
	Content  string `json:"content"`
	Status   string `json:"status"`
	Priority string `json:"priority"`
}

type Input struct {
	Todos   []TodoItem `json:"todos"`
	Merge   bool       `json:"merge"`
	Summary string     `json:"summary,omitempty"`
}

type TodoWriteTool struct{}

func New() *TodoWriteTool { return &TodoWriteTool{} }

func (t *TodoWriteTool) Name() string { return "TodoWrite" }

func (t *TodoWriteTool) Prompt() string {
	return strings.TrimSpace(`- Updates a todo list stored in the workspace.
- Use to track multi-step work.
- merge=true updates existing items by id; merge=false replaces the list.`)
}

func (t *TodoWriteTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type":"object",
  "additionalProperties":false,
  "properties":{
    "todos":{
      "type":"array",
      "items":{
        "type":"object",
        "additionalProperties":false,
        "properties":{
          "id":{"type":"string"},
          "content":{"type":"string"},
          "status":{"type":"string","enum":["pending","in_progress","completed"]},
          "priority":{"type":"string","enum":["high","medium","low"]}
        },
        "required":["id","content","status","priority"]
      }
    },
    "merge":{"type":"boolean"},
    "summary":{"type":"string"}
  },
  "required":["todos","merge"]
}`)
}

func (t *TodoWriteTool) IsReadOnly(_ any) bool        { return false }
func (t *TodoWriteTool) IsConcurrencySafe(_ any) bool { return false }

func (t *TodoWriteTool) ValidateInput(input any) error {
	switch v := input.(type) {
	case Input:
		if v.Todos == nil {
			return fmt.Errorf("todos is required")
		}
	}
	return nil
}

func (t *TodoWriteTool) CheckPermissions(_ context.Context, _ any, _ tool.PermissionContext) (tool.PermissionDecision, any, error) {
	// Consider todo updates low-risk; allow by default.
	return tool.PermissionDecision{Behavior: tool.PermissionBehaviorAllow}, nil, nil
}

func loadTodos(path string) ([]TodoItem, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var items []TodoItem
	if err := json.Unmarshal(b, &items); err != nil {
		return nil, err
	}
	return items, nil
}

func saveTodos(path string, items []TodoItem) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(items, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func (t *TodoWriteTool) Call(_ context.Context, input any) (tool.ToolResult, error) {
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

	path := ".claude-go/todo.json"

	var out []TodoItem
	if in.Merge {
		existing, err := loadTodos(path)
		if err != nil {
			return tool.ToolResult{IsError: true, Content: err.Error()}, nil
		}
		byID := map[string]TodoItem{}
		for _, it := range existing {
			byID[it.ID] = it
		}
		for _, it := range in.Todos {
			byID[it.ID] = it
		}
		out = make([]TodoItem, 0, len(byID))
		for _, it := range byID {
			out = append(out, it)
		}
	} else {
		out = in.Todos
	}

	if err := saveTodos(path, out); err != nil {
		return tool.ToolResult{IsError: true, Content: err.Error()}, nil
	}
	msg := fmt.Sprintf("Todo list updated (%d items) at %s", len(out), path)
	if strings.TrimSpace(in.Summary) != "" {
		msg += "\nSummary: " + strings.TrimSpace(in.Summary)
	}
	return tool.ToolResult{Content: msg}, nil
}

