package query

import (
	"context"

	"claude-code-running-go/src/services/api"
)

type StopHookResult struct {
	Messages             []api.Message
	PreventContinuation  bool
}

type StopHookRunner interface {
	AfterTurn(ctx context.Context, msgs []api.Message, assistantContent []api.ContentBlock) (StopHookResult, error)
}

type NoopStopHookRunner struct{}

func (NoopStopHookRunner) AfterTurn(_ context.Context, _ []api.Message, _ []api.ContentBlock) (StopHookResult, error) {
	return StopHookResult{}, nil
}

