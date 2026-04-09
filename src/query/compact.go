package query

import (
	"fmt"

	"claude-code-running-go/src/services/api"
)

type CompactResult struct {
	Triggered      bool
	OmittedCount   int
	EstimatedTokens int
}

func maybeCompactMessages(msgs []api.Message, tokenThreshold int, keepLast int) ([]api.Message, *CompactResult) {
	if tokenThreshold <= 0 || len(msgs) == 0 {
		return msgs, nil
	}
	est := approximateTokenCount(msgs)
	if est < tokenThreshold {
		return msgs, nil
	}
	if keepLast <= 0 {
		keepLast = 12
	}
	if len(msgs) <= keepLast {
		return msgs, nil
	}

	omitUntil := len(msgs) - keepLast
	if omitUntil < 1 {
		return msgs, nil
	}
	omitted := msgs[:omitUntil]
	kept := append([]api.Message{}, msgs[omitUntil:]...)

	summary := api.Message{
		Role: "user",
		Content: fmt.Sprintf(
			"<system-reminder>Earlier conversation was compacted for budget. %d messages were omitted; continue using the surviving transcript state and current files as source of truth.</system-reminder>",
			len(omitted),
		),
	}
	out := make([]api.Message, 0, len(kept)+1)
	out = append(out, summary)
	out = append(out, kept...)
	return out, &CompactResult{
		Triggered:      true,
		OmittedCount:   len(omitted),
		EstimatedTokens: est,
	}
}

