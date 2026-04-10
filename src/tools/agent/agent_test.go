package agent

import (
	"context"
	"testing"
)

func TestAgentTool_AdhocDoesNotRequireResolver(t *testing.T) {
	var got RunOptions
	tool := NewWithConfig(Config{
		BaseSystemPrompt: []string{"base"},
		ParentModel:      "sonnet",
		ParentMode:       "default",
		Resolve:          nil, // should not be required for ad-hoc
		Run: func(ctx context.Context, opts RunOptions) (string, error) {
			_ = ctx
			got = opts
			return "ok", nil
		},
		DepthLimit: 1,
	})

	res, err := tool.Call(context.Background(), map[string]any{
		"description":       "adhoc test",
		"prompt":            "do a thing",
		"response_language": "zh",
		"system_prompt":     "You are an ad-hoc agent",
		"model":             "haiku",
		"permission_mode":   "bypass",
		"max_turns":         2,
		"tools":             []any{"Read", "Grep"},
		"disallowed_tools":  []any{"Bash"},
	})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if res.IsError {
		t.Fatalf("expected success, got error: %v", res.Content)
	}
	if got.Model != "haiku" {
		t.Fatalf("model override not applied: %q", got.Model)
	}
	if got.PermissionMode != "bypass" {
		t.Fatalf("permission override not applied: %q", got.PermissionMode)
	}
	if got.MaxTurns != 2 {
		t.Fatalf("maxTurns override not applied: %d", got.MaxTurns)
	}
	if len(got.SystemPrompt) < 2 {
		t.Fatalf("expected base+systemPrompt, got: %#v", got.SystemPrompt)
	}
}

