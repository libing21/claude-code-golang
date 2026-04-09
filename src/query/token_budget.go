package query

import (
	"encoding/json"
	"fmt"
	"time"

	"claude-code-running-go/src/services/api"
)

const (
	completionThresholdPct = 0.9
	diminishingThreshold   = 500
)

type BudgetTracker struct {
	ContinuationCount int
	LastDeltaTokens   int
	LastGlobalTokens  int
	StartedAt         time.Time
}

func NewBudgetTracker() *BudgetTracker {
	return &BudgetTracker{StartedAt: time.Now()}
}

type TokenBudgetDecision struct {
	Action              string
	NudgeMessage        string
	ContinuationCount   int
	Pct                 int
	TurnTokens          int
	Budget              int
	DiminishingReturns  bool
	CompletionEventText string
}

func approximateTokenCount(msgs []api.Message) int {
	b, err := json.Marshal(msgs)
	if err != nil {
		return 0
	}
	// Match TS rough heuristic directionally: ~4 bytes per token.
	return len(b) / 4
}

func checkTokenBudget(tracker *BudgetTracker, budget int, globalTurnTokens int) TokenBudgetDecision {
	if tracker == nil || budget <= 0 {
		return TokenBudgetDecision{Action: "stop"}
	}

	pct := 0
	if budget > 0 {
		pct = int(float64(globalTurnTokens) / float64(budget) * 100)
	}
	deltaSinceLastCheck := globalTurnTokens - tracker.LastGlobalTokens
	isDiminishing := tracker.ContinuationCount >= 3 &&
		deltaSinceLastCheck < diminishingThreshold &&
		tracker.LastDeltaTokens < diminishingThreshold

	if !isDiminishing && float64(globalTurnTokens) < float64(budget)*completionThresholdPct {
		tracker.ContinuationCount++
		tracker.LastDeltaTokens = deltaSinceLastCheck
		tracker.LastGlobalTokens = globalTurnTokens
		return TokenBudgetDecision{
			Action:            "continue",
			NudgeMessage:      fmt.Sprintf("Continue working until the task is fully complete. Current budget usage: ~%d%% (%d/%d tokens).", pct, globalTurnTokens, budget),
			ContinuationCount: tracker.ContinuationCount,
			Pct:               pct,
			TurnTokens:        globalTurnTokens,
			Budget:            budget,
		}
	}

	if isDiminishing || tracker.ContinuationCount > 0 {
		return TokenBudgetDecision{
			Action:             "stop",
			Pct:                pct,
			TurnTokens:         globalTurnTokens,
			Budget:             budget,
			DiminishingReturns: isDiminishing,
			CompletionEventText: fmt.Sprintf(
				"Token budget complete after %d continuation(s): ~%d%% (%d/%d tokens), diminishing=%v, duration_ms=%d.",
				tracker.ContinuationCount,
				pct,
				globalTurnTokens,
				budget,
				isDiminishing,
				time.Since(tracker.StartedAt).Milliseconds(),
			),
		}
	}

	return TokenBudgetDecision{Action: "stop"}
}

