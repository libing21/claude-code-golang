package attachments

import (
	"encoding/json"
	"strings"

	"claude-code-running-go/src/services/api"
)

const KindAgentListingDelta Kind = "agent_listing_delta"

type AgentListingDeltaProvider struct{}

func (p AgentListingDeltaProvider) Kind() Kind    { return KindAgentListingDelta }
func (p AgentListingDeltaProvider) Priority() int { return 45 }

func (p AgentListingDeltaProvider) Build(ctx Context) []api.Message {
	a := ctx.AgentListingDelta
	if a == nil {
		return nil
	}
	parts := make([]string, 0, 3)
	if len(a.AddedLines) > 0 {
		header := "New agent types are now available for the Agent tool:"
		if a.IsInitial {
			header = "Available agent types for the Agent tool:"
		}
		parts = append(parts, header+"\n"+strings.Join(a.AddedLines, "\n"))
	}
	if len(a.RemovedTypes) > 0 {
		lines := make([]string, 0, len(a.RemovedTypes))
		for _, t := range a.RemovedTypes {
			lines = append(lines, "- "+t)
		}
		parts = append(parts, "The following agent types are no longer available:\n"+strings.Join(lines, "\n"))
	}
	if a.IsInitial && a.ShowConcurrencyNote {
		parts = append(parts, "Launch multiple agents concurrently whenever possible, to maximize performance; to do that, use a single message with multiple tool uses.")
	}
	if len(parts) == 0 {
		return nil
	}
	state, _ := json.Marshal(map[string]any{
		"type":         "agent_listing_delta",
		"addedTypes":   a.AddedTypes,
		"removedTypes": a.RemovedTypes,
	})
	payload := "\n<agent_listing_delta_json>" + string(state) + "</agent_listing_delta_json>"
	return []api.Message{{Role: "user", Content: "<system-reminder>\n" + strings.Join(parts, "\n\n") + payload + "\n</system-reminder>"}}
}
