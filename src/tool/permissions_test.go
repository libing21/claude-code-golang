package tool

import (
	"context"
	"encoding/json"
	"testing"
)

type fakeTool struct {
	name string
}

func (f fakeTool) Name() string                           { return f.name }
func (f fakeTool) Prompt() string                         { return "" }
func (f fakeTool) InputSchema() json.RawMessage           { return json.RawMessage(`{}`) }
func (f fakeTool) IsReadOnly(input any) bool              { return true }
func (f fakeTool) IsConcurrencySafe(input any) bool       { return true }
func (f fakeTool) ValidateInput(input any) error          { return nil }
func (f fakeTool) Call(ctx context.Context, input any) (ToolResult, error) {
	return ToolResult{Content: "ok"}, nil
}
func (f fakeTool) CheckPermissions(ctx context.Context, input any, permCtx PermissionContext) (PermissionDecision, any, error) {
	return PermissionDecision{Behavior: PermissionBehaviorAllow}, nil, nil
}

func TestPermissionRuleValueFromString(t *testing.T) {
	got := PermissionRuleValueFromString("Bash(git status)")
	if got.ToolName != "Bash" || got.RuleContent != "git status" {
		t.Fatalf("unexpected parse result: %+v", got)
	}
}

func TestResolvePermissionDecisionDenyWins(t *testing.T) {
	permCtx := PermissionContext{
		Mode: PermissionModeDefault,
		DenyRules: BuildRulesFromStrings(
			"cliArg",
			PermissionBehaviorDeny,
			[]string{"Read"},
		),
	}
	dec, _, err := ResolvePermissionDecision(context.Background(), fakeTool{name: "Read"}, nil, permCtx)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if dec.Behavior != PermissionBehaviorDeny {
		t.Fatalf("expected deny, got %q", dec.Behavior)
	}
}

func TestResolvePermissionDecisionAllowRule(t *testing.T) {
	permCtx := PermissionContext{
		Mode: PermissionModeDefault,
		AllowRules: BuildRulesFromStrings(
			"cliArg",
			PermissionBehaviorAllow,
			[]string{"Read"},
		),
	}
	dec, _, err := ResolvePermissionDecision(context.Background(), fakeTool{name: "Read"}, nil, permCtx)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if dec.Behavior != PermissionBehaviorAllow {
		t.Fatalf("expected allow, got %q", dec.Behavior)
	}
}

