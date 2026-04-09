package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"claude-code-running-go/src/tool"
)

// Agent is a minimal runtime analog of TS AgentTool.
// It runs a nested "one-shot" tool loop using injected runner+resolver hooks.

type Input struct {
	Description      string `json:"description"`
	Prompt           string `json:"query"`
	SubagentType     string `json:"subagent_type"`
	ResponseLanguage string `json:"response_language"`
}

type AgentSpec struct {
	AgentType          string
	SystemPrompt       string
	Tools              []string
	DisallowedTools    []string
	Model              string
	PermissionMode     string
	MaxTurns           int
	RequiredMcpServers []string
}

type ResolveFunc func(agentType string) (AgentSpec, bool)

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
	BaseSystemPrompt []string
	ParentModel      string
	ParentMode       string
	Resolve          ResolveFunc
	Run              RunnerFunc
	DepthLimit       int
}

type AgentTool struct {
	cfg                 Config
	availableMcpServers []string
	depth               int
}

func New() *AgentTool { return &AgentTool{} } // for toolnames; not executable without config

func NewWithConfig(cfg Config) *AgentTool {
	if cfg.DepthLimit <= 0 {
		cfg.DepthLimit = 1
	}
	return &AgentTool{cfg: cfg}
}

func (t *AgentTool) SetAvailableMcpServers(servers []string) {
	t.availableMcpServers = servers
}

func (t *AgentTool) Name() string { return "Agent" }

func (t *AgentTool) Prompt() string {
	return "Launch a specialized subagent to perform a task."
}

func (t *AgentTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type":"object",
  "properties":{
    "description":{"type":"string"},
    "query":{"type":"string"},
    "subagent_type":{"type":"string"},
    "response_language":{"type":"string"}
  },
  "required":["description","query","subagent_type","response_language"],
  "additionalProperties":false
}`)
}

func (t *AgentTool) IsReadOnly(_ any) bool        { return true }
func (t *AgentTool) IsConcurrencySafe(_ any) bool { return true }
func (t *AgentTool) ValidateInput(_ any) error    { return nil }

func (t *AgentTool) CheckPermissions(_ context.Context, _ any, _ tool.PermissionContext) (tool.PermissionDecision, any, error) {
	// Defer to outer permission rules; this can be expensive and may require explicit allow.
	return tool.PermissionDecision{Behavior: tool.PermissionBehaviorPassthrough}, nil, nil
}

func (t *AgentTool) Call(ctx context.Context, input any) (tool.ToolResult, error) {
	if t.cfg.Resolve == nil || t.cfg.Run == nil {
		return tool.ToolResult{IsError: true, Content: "Agent tool is not configured (missing resolver/runner)."}, nil
	}
	if t.depth >= t.cfg.DepthLimit {
		return tool.ToolResult{IsError: true, Content: "Nested Agent calls are not supported in this Go port yet."}, nil
	}
	in := Input{}
	b, _ := json.Marshal(input)
	_ = json.Unmarshal(b, &in)
	in.SubagentType = strings.TrimSpace(in.SubagentType)
	if in.SubagentType == "" {
		in.SubagentType = "general_purpose_task"
	}
	prompt := strings.TrimSpace(in.Prompt)
	if prompt == "" {
		return tool.ToolResult{IsError: true, Content: "query is required"}, nil
	}

	spec, ok := t.cfg.Resolve(in.SubagentType)
	if !ok {
		return tool.ToolResult{IsError: true, Content: "Unknown subagent_type: " + in.SubagentType}, nil
	}

	// requiredMcpServers gate (TS: hasRequiredMcpServers + filterAgentsByMcpRequirements).
	if len(spec.RequiredMcpServers) > 0 {
		for _, pat := range spec.RequiredMcpServers {
			pat = strings.ToLower(strings.TrimSpace(pat))
			if pat == "" {
				continue
			}
			found := false
			for _, s := range t.availableMcpServers {
				if strings.Contains(strings.ToLower(s), pat) {
					found = true
					break
				}
			}
			if !found {
				return tool.ToolResult{IsError: true, Content: fmt.Sprintf("Agent '%s' requires MCP server matching '%s', but it is not available.", spec.AgentType, pat)}, nil
			}
		}
	}

	// Model/permission/turns inherit rules.
	model := strings.TrimSpace(spec.Model)
	if model == "" || strings.EqualFold(model, "inherit") {
		model = strings.TrimSpace(t.cfg.ParentModel)
	}
	mode := strings.TrimSpace(spec.PermissionMode)
	if mode == "" {
		mode = strings.TrimSpace(t.cfg.ParentMode)
	}
	maxTurns := spec.MaxTurns
	if maxTurns <= 0 {
		maxTurns = 8
	}

	// Tool pool restrictions:
	// - tools nil => inherit "all tools" (leave AllowedTools empty to mean no restriction)
	// - tools [] => no tools (AllowedTools empty slice but with explicit marker)
	allowed := spec.Tools
	disallowed := append([]string{}, spec.DisallowedTools...)

	// Avoid recursive spawning unless explicitly allowed.
	if t.depth > 0 {
		disallowed = append(disallowed, "Agent")
	}

	sys := append([]string{}, t.cfg.BaseSystemPrompt...)
	if strings.TrimSpace(spec.SystemPrompt) != "" {
		sys = append(sys, strings.TrimSpace(spec.SystemPrompt))
	}

	t.depth++
	out, err := t.cfg.Run(ctx, RunOptions{
		SystemPrompt:    sys,
		UserPrompt:      prompt,
		Model:           model,
		PermissionMode:  mode,
		AllowedTools:    allowed,
		DisallowedTools: disallowed,
		MaxTurns:        maxTurns,
		IsSubAgent:      true,
	})
	t.depth--
	if err != nil {
		return tool.ToolResult{IsError: true, Content: err.Error()}, nil
	}
	return tool.ToolResult{Content: out}, nil
}
