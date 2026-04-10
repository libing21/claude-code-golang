package discoverskills

import (
	"context"
	"encoding/json"
	"strings"

	"claude-code-running-go/src/tool"
	skilltool "claude-code-running-go/src/tools/skill"
)

// DiscoverSkills is a minimal Go analog of TS DiscoverSkills tool.
// It lists installed skills found in configured skill directories.

type Input struct {
	Description string `json:"description"`
}

type DiscoverSkillsTool struct {
	roots []string
}

func New(roots []string) *DiscoverSkillsTool { return &DiscoverSkillsTool{roots: roots} }

func (t *DiscoverSkillsTool) Name() string { return "DiscoverSkills" }

func (t *DiscoverSkillsTool) Prompt() string {
	return "Discover available skills relevant to a described task."
}

func (t *DiscoverSkillsTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object","properties":{"description":{"type":"string"}},"required":["description"],"additionalProperties":false}`)
}

func (t *DiscoverSkillsTool) IsReadOnly(_ any) bool        { return true }
func (t *DiscoverSkillsTool) IsConcurrencySafe(_ any) bool { return true }
func (t *DiscoverSkillsTool) ValidateInput(_ any) error    { return nil }

func (t *DiscoverSkillsTool) CheckPermissions(_ context.Context, _ any, _ tool.PermissionContext) (tool.PermissionDecision, any, error) {
	return tool.PermissionDecision{Behavior: tool.PermissionBehaviorAllow}, nil, nil
}

func (t *DiscoverSkillsTool) Call(_ context.Context, input any) (tool.ToolResult, error) {
	_ = input
	skills := skilltool.ListSkills(t.roots)
	if len(skills) == 0 {
		return tool.ToolResult{Content: "No skill directories configured."}, nil
	}
	lines := make([]string, 0, len(skills))
	for _, s := range skills {
		line := "- " + s.Name
		if desc := strings.TrimSpace(s.Description); desc != "" {
			line += " — " + desc
		} else if s.Kind == "markdown" {
			line += " — standard SKILL.md"
		} else {
			line += " — script skill"
		}
		lines = append(lines, line)
	}
	return tool.ToolResult{Content: "Discovered skills:\n" + strings.Join(lines, "\n")}, nil
}
