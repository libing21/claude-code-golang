package query

import (
	"context"
	"testing"

	"claude-code-running-go/src/services/api"
)

type fakeStopHookRunner struct {
	result StopHookResult
}

func (f fakeStopHookRunner) AfterTurn(_ context.Context, _ []api.Message, _ []api.ContentBlock) (StopHookResult, error) {
	return f.result, nil
}

func TestMaybeCompactMessages(t *testing.T) {
	msgs := []api.Message{
		{Role: "user", Content: "a"},
		{Role: "assistant", Content: "b"},
		{Role: "user", Content: "c"},
		{Role: "assistant", Content: "d"},
	}
	compacted, info := maybeCompactMessages(msgs, 1, 2)
	if info == nil || !info.Triggered {
		t.Fatalf("expected compaction to trigger")
	}
	if len(compacted) != 3 {
		t.Fatalf("expected compacted length 3, got %d", len(compacted))
	}
}

func TestCheckTokenBudgetContinue(t *testing.T) {
	tracker := NewBudgetTracker()
	decision := checkTokenBudget(tracker, 1000, 100)
	if decision.Action != "continue" {
		t.Fatalf("expected continue, got %s", decision.Action)
	}
	if decision.NudgeMessage == "" {
		t.Fatalf("expected nudge message")
	}
}

func TestNoopStopHookRunner(t *testing.T) {
	var r StopHookRunner = fakeStopHookRunner{
		result: StopHookResult{
			Messages:            []api.Message{{Role: "user", Content: "hook"}},
			PreventContinuation: true,
		},
	}
	out, err := r.AfterTurn(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("after turn: %v", err)
	}
	if !out.PreventContinuation || len(out.Messages) != 1 {
		t.Fatalf("unexpected hook result: %+v", out)
	}
}

