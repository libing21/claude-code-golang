package discoverskills

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"claude-code-running-go/src/tool"
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
	roots := make([]string, 0, 4)
	if v := strings.TrimSpace(os.Getenv("CLAUDE_GO_SKILLS_DIR")); v != "" {
		roots = append(roots, v)
	}
	roots = append(roots, t.roots...)
	if len(roots) == 0 {
		return tool.ToolResult{Content: "No skill directories configured."}, nil
	}

	names := make([]string, 0, 32)
	seen := map[string]struct{}{}
	for _, r := range roots {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		entries, err := os.ReadDir(r)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			name := e.Name()
			if name == "" {
				continue
			}
			// Require either <skill>/run.sh or <skill> executable.
			if _, err := os.Stat(filepath.Join(r, name, "run.sh")); err == nil {
				if _, ok := seen[name]; !ok {
					seen[name] = struct{}{}
					names = append(names, name)
				}
				continue
			}
			if _, err := os.Stat(filepath.Join(r, name)); err == nil {
				if _, ok := seen[name]; !ok {
					seen[name] = struct{}{}
					names = append(names, name)
				}
				continue
			}
		}
	}
	sort.Strings(names)
	if len(names) == 0 {
		return tool.ToolResult{Content: "No skills found."}, nil
	}
	return tool.ToolResult{Content: "Discovered skills:\n- " + strings.Join(names, "\n- ")}, nil
}

