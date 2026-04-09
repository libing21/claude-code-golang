package attachments

import (
	"encoding/json"
	"strings"

	"claude-code-running-go/src/services/api"
)

const KindDeferredToolsDelta Kind = "deferred_tools_delta"

type DeferredToolsDeltaProvider struct{}

func (p DeferredToolsDeltaProvider) Kind() Kind    { return KindDeferredToolsDelta }
func (p DeferredToolsDeltaProvider) Priority() int { return 44 }

func (p DeferredToolsDeltaProvider) Build(ctx Context) []api.Message {
	a := ctx.DeferredToolsDelta
	if a == nil {
		return nil
	}
	parts := make([]string, 0, 2)
	if len(a.AddedLines) > 0 {
		parts = append(parts, "The following deferred tools are now available via ToolSearch:\n"+strings.Join(a.AddedLines, "\n"))
	}
	if len(a.RemovedNames) > 0 {
		parts = append(parts, "The following deferred tools are no longer available (their MCP server disconnected). Do not search for them — ToolSearch will return no match:\n"+strings.Join(a.RemovedNames, "\n"))
	}
	if len(parts) == 0 {
		return nil
	}
	state, _ := json.Marshal(map[string]any{
		"type":         "deferred_tools_delta",
		"addedNames":   a.AddedNames,
		"removedNames": a.RemovedNames,
	})
	payload := "\n<deferred_tools_delta_json>" + string(state) + "</deferred_tools_delta_json>"
	return []api.Message{{Role: "user", Content: "<system-reminder>\n" + strings.Join(parts, "\n\n") + payload + "\n</system-reminder>"}}
}
