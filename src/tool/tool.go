package tool

import (
	"context"
	"encoding/json"
)

// Tool is a Go analog of the TS Tool interface (src/Tool.ts), simplified for phase-1.
// It is intentionally explicit and "protocol-shaped" for easy debugging.
type Tool interface {
	Name() string
	Prompt() string
	InputSchema() json.RawMessage // JSON Schema object
	IsReadOnly(input any) bool
	IsConcurrencySafe(input any) bool
	ValidateInput(input any) error
	CheckPermissions(ctx context.Context, input any, permCtx PermissionContext) (PermissionDecision, any, error)
	Call(ctx context.Context, input any) (ToolResult, error)
}

type ToolResult struct {
	// Content can be:
	// - string (common)
	// - []any (block tool_result content, e.g. tool_reference blocks)
	Content any
	IsError bool
}

type PermissionMode string

const (
	PermissionModeDefault PermissionMode = "default"
	PermissionModeAsk     PermissionMode = "ask"
	PermissionModeBypass  PermissionMode = "bypass"
)

type PermissionBehavior string

const (
	PermissionBehaviorAllow       PermissionBehavior = "allow"
	PermissionBehaviorDeny        PermissionBehavior = "deny"
	PermissionBehaviorAsk         PermissionBehavior = "ask"
	PermissionBehaviorPassthrough PermissionBehavior = "passthrough"
)

type PermissionRuleValue struct {
	ToolName    string
	RuleContent string
}

type PermissionRule struct {
	Source       string
	RuleBehavior PermissionBehavior
	RuleValue    PermissionRuleValue
}

// PermissionContext is the start of a TS-like permission runtime context.
type PermissionContext struct {
	Mode       PermissionMode
	AllowRules []PermissionRule
	DenyRules  []PermissionRule
	AskRules   []PermissionRule
}

type PermissionDecision struct {
	Behavior PermissionBehavior // "allow" | "deny" | "ask"
	Reason   string
	CheckID  int    // 0 means unset; for UI/log parity with TS numeric check IDs
	CheckTag string // optional stable tag for UI/log filtering
}
