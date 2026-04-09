package query

import (
	"encoding/json"
	"strings"

	"claude-code-running-go/src/services/api"
)

func extractJSONTag(msgs []api.Message, tag string) []string {
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	out := make([]string, 0)
	for _, m := range msgs {
		s, ok := m.Content.(string)
		if !ok {
			continue
		}
		start := 0
		for {
			i := strings.Index(s[start:], open)
			if i < 0 {
				break
			}
			i += start + len(open)
			j := strings.Index(s[i:], close)
			if j < 0 {
				break
			}
			j += i
			out = append(out, s[i:j])
			start = j + len(close)
		}
	}
	return out
}

func scanAnnouncedAgentTypes(msgs []api.Message) map[string]struct{} {
	type payload struct {
		AddedTypes   []string `json:"addedTypes"`
		RemovedTypes []string `json:"removedTypes"`
	}
	out := map[string]struct{}{}
	for _, raw := range extractJSONTag(msgs, "agent_listing_delta_json") {
		var p payload
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			continue
		}
		for _, t := range p.AddedTypes {
			out[t] = struct{}{}
		}
		for _, t := range p.RemovedTypes {
			delete(out, t)
		}
	}
	return out
}

func scanAnnouncedDeferredTools(msgs []api.Message) map[string]struct{} {
	type payload struct {
		AddedNames   []string `json:"addedNames"`
		RemovedNames []string `json:"removedNames"`
	}
	out := map[string]struct{}{}
	for _, raw := range extractJSONTag(msgs, "deferred_tools_delta_json") {
		var p payload
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			continue
		}
		for _, n := range p.AddedNames {
			out[n] = struct{}{}
		}
		for _, n := range p.RemovedNames {
			delete(out, n)
		}
	}
	return out
}

