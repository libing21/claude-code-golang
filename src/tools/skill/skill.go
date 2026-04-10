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
	Args string `json:"args"`
}

type RunOptions struct {
	SystemPrompt    []string
	UserPrompt      string
	Model           string
	PermissionMode  string
	AllowedTools    []string
	DisallowedTools []string
	MaxTurns        int
	IsSubAgent      bool
}

type RunnerFunc func(ctx context.Context, opts RunOptions) (string, error)

type Config struct {
	Dirs             []string
	BaseSystemPrompt []string
	ParentModel      string
	ParentMode       string
	Run              RunnerFunc
}

type SkillTool struct {
	cfg Config
}

func New(dirs []string) *SkillTool { return &SkillTool{cfg: Config{Dirs: dirs}} }

func NewWithConfig(cfg Config) *SkillTool { return &SkillTool{cfg: cfg} }

func (t *SkillTool) Name() string { return "Skill" }

func (t *SkillTool) Prompt() string {
	return strings.TrimSpace(`- Executes an installed skill by name.
- Skills are external capability modules.
- Standard skills are loaded from <skill>/SKILL.md and executed in a forked sub-agent.
- Legacy script skills (<skill>/run.sh) are still supported as a fallback.`)
}

func (t *SkillTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type":"object",
  "additionalProperties":false,
  "properties":{
    "name":{"type":"string","description":"Skill name"},
    "args":{"type":"string","description":"Optional arguments for the skill"}
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
	roots = append(roots, configuredRoots(t.cfg.Dirs)...)
	if len(roots) == 0 {
		return tool.ToolResult{IsError: true, Content: "no skill dirs configured: set CLAUDE_GO_SKILLS_DIR or pass --skill-dir"}, nil
	}

	def, ok := ResolveSkill(t.cfg.Dirs, in.Name)
	if !ok {
		return tool.ToolResult{IsError: true, Content: "skill not found: " + in.Name}, nil
	}

	if def.Kind == "markdown" {
		if t.cfg.Run == nil {
			return tool.ToolResult{IsError: true, Content: "skill runtime is not configured (missing runner)"}, nil
		}
		finalContent := buildSkillPromptContent(def)

		model := strings.TrimSpace(def.Model)
		if model == "" || strings.EqualFold(model, "inherit") {
			model = strings.TrimSpace(t.cfg.ParentModel)
		}
		permissionMode := strings.TrimSpace(def.PermissionMode)
		if permissionMode == "" {
			permissionMode = strings.TrimSpace(t.cfg.ParentMode)
		}
		maxTurns := def.MaxTurns
		if maxTurns <= 0 {
			maxTurns = 4
		}
		userPrompt := strings.TrimSpace(in.Args)
		if userPrompt == "" {
			userPrompt = "Execute this skill and return the result."
		}
		out, err := t.cfg.Run(ctx, RunOptions{
			SystemPrompt:    append(append([]string{}, t.cfg.BaseSystemPrompt...), finalContent),
			UserPrompt:      userPrompt,
			Model:           model,
			PermissionMode:  permissionMode,
			AllowedTools:    def.Tools,
			DisallowedTools: def.DisallowedTools,
			MaxTurns:        maxTurns,
			IsSubAgent:      true,
		})
		if err != nil {
			return tool.ToolResult{IsError: true, Content: "skill failed: " + err.Error()}, nil
		}
		return tool.ToolResult{Content: out}, nil
	}

	entry := def.Entry
	if entry == "" {
		entry = filepath.Join(def.Dir, in.Name, "run.sh")
	}
	cmd := exec.CommandContext(ctx, "bash", "-lc", shellEscape(entry)+" "+shellEscape(strings.TrimSpace(in.Args)))
	cmd.Env = append(os.Environ(), "PAGER=cat")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return tool.ToolResult{IsError: true, Content: fmt.Sprintf("skill failed: %s\n%s", err.Error(), string(out))}, nil
	}
	return tool.ToolResult{Content: string(out)}, nil
}

func buildSkillPromptContent(def Definition) string {
	body := strings.TrimSpace(def.Body)
	dir := filepath.Clean(def.Dir)
	dirForPrompt := filepath.ToSlash(dir)
	if dirForPrompt == "." || dirForPrompt == "" {
		dirForPrompt = dir
	}
	body = strings.ReplaceAll(body, "${CLAUDE_SKILL_DIR}", dirForPrompt)
	sessionID := strings.TrimSpace(os.Getenv("CLAUDE_SESSION_ID"))
	if sessionID == "" {
		sessionID = "go-session"
	}
	body = strings.ReplaceAll(body, "${CLAUDE_SESSION_ID}", sessionID)
	return "Base directory for this skill: " + dirForPrompt + "\n\n" + body
}

func shellEscape(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}
