package query

import (
	"strings"

	"claude-code-running-go/src/services/api"
)

type ToolBudgetState struct {
	SeenIDs      map[string]struct{}
	Replacements map[string]string
}

func NewToolBudgetState() *ToolBudgetState {
	return &ToolBudgetState{
		SeenIDs:      map[string]struct{}{},
		Replacements: map[string]string{},
	}
}

func ReconstructToolBudgetState(msgs []api.Message) *ToolBudgetState {
	st := NewToolBudgetState()
	for _, msg := range msgs {
		if msg.Role != "user" {
			continue
		}
		blocks, ok := msg.Content.([]any)
		if ok {
			st.consumeAnyBlocks(blocks)
			continue
		}
		if blocks, ok := msg.Content.([]map[string]any); ok {
			tmp := make([]any, 0, len(blocks))
			for _, b := range blocks {
				tmp = append(tmp, b)
			}
			st.consumeAnyBlocks(tmp)
		}
	}
	return st
}

func (s *ToolBudgetState) consumeAnyBlocks(blocks []any) {
	for _, it := range blocks {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		typ, _ := m["type"].(string)
		if typ != "tool_result" {
			continue
		}
		id, _ := m["tool_use_id"].(string)
		if id == "" {
			continue
		}
		s.SeenIDs[id] = struct{}{}
		content, _ := m["content"].(string)
		if isToolBudgetReplacement(content) {
			s.Replacements[id] = content
		}
	}
}

func isToolBudgetReplacement(content string) bool {
	content = strings.TrimSpace(content)
	if content == "" {
		return false
	}
	if strings.Contains(content, "Full content saved to") && strings.Contains(content, "Result truncated") {
		return true
	}
	if strings.HasPrefix(content, "<persisted-output>") {
		return true
	}
	return false
}

