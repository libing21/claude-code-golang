package query

import (
	"strings"

	"claude-code-running-go/src/services/api"
)

const compactMarkerSubstring = "Earlier conversation was compacted for budget."

func applyLastCompactionMarker(msgs []api.Message) []api.Message {
	last := -1
	for i, m := range msgs {
		if m.Role != "user" {
			continue
		}
		s, ok := m.Content.(string)
		if !ok {
			continue
		}
		if strings.Contains(s, compactMarkerSubstring) {
			last = i
		}
	}
	if last >= 0 {
		return append([]api.Message{}, msgs[last:]...)
	}
	return msgs
}

