package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"claude-code-running-go/src/tool"
)

type Input struct {
	Name string `json:"name"`
}

type SkillTool struct {
	dirs []string
}

func New(dirs []string) *SkillTool { return &SkillTool{dirs: dirs} }

func (t *SkillTool) Name() string { return "Skill" }

func (t *SkillTool) Prompt() string {
	return strings.TrimSpace(`- Executes an installed skill by name.
- Skills are external capability modules.
- In Go port, skills are loaded from CLAUDE_GO_SKILLS_DIR when configured.`)
}

func (t *SkillTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type":"object",
  "additionalProperties":false,
  "properties":{
    "name":{"type":"string","description":"Skill name"}
  },
  "required":["name"]
}`)
}

func (t *SkillTool) IsReadOnly(_ any) bool        { return false }
func (t *SkillTool) IsConcurrencySafe(_ any) bool { return false }

func (t *SkillTool) ValidateInput(input any) error {
	switch v := input.(type) {
	case Input:
		if strings.TrimSpace(v.Name) == "" {
			return fmt.Errorf("name is required")
		}
	}
	return nil
}

func (t *SkillTool) CheckPermissions(_ context.Context, _ any, _ tool.PermissionContext) (tool.PermissionDecision, any, error) {
	// Treat as potentially dangerous: rely on outer permission.
	return tool.PermissionDecision{Behavior: tool.PermissionBehaviorPassthrough}, nil, nil
}

func (t *SkillTool) Call(ctx context.Context, input any) (tool.ToolResult, error) {
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

	roots := make([]string, 0, 4)
	if v := strings.TrimSpace(os.Getenv("CLAUDE_GO_SKILLS_DIR")); v != "" {
		roots = append(roots, v)
	}
	roots = append(roots, t.dirs...)
	if len(roots) == 0 {
		return tool.ToolResult{IsError: true, Content: "no skill dirs configured: set CLAUDE_GO_SKILLS_DIR or pass --skill-dir"}, nil
	}

	var entry string
	for _, dir := range roots {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		cand := filepath.Join(dir, in.Name, "run.sh")
		if _, err := os.Stat(cand); err == nil {
			entry = cand
			break
		}
		cand = filepath.Join(dir, in.Name)
		if _, err := os.Stat(cand); err == nil {
			entry = cand
			break
		}
	}
	if entry == "" {
		return tool.ToolResult{IsError: true, Content: "skill not found: " + in.Name}, nil
	}

	cmd := exec.CommandContext(ctx, "bash", "-lc", entry)
	cmd.Env = append(os.Environ(), "PAGER=cat")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return tool.ToolResult{IsError: true, Content: fmt.Sprintf("skill failed: %s\n%s", err.Error(), string(out))}, nil
	}
	return tool.ToolResult{Content: string(out)}, nil
}
