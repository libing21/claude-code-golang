package api

import (
	"encoding/json"
	"fmt"
)

// StreamAccumulator builds a MessagesResponse from RawMessageStreamEvent stream.
// It supports the subset needed by QueryEngine: text blocks and tool_use blocks.
type StreamAccumulator struct {
	resp       MessagesResponse
	content    []ContentBlock
	finalized  map[int]bool
}

func NewStreamAccumulator() *StreamAccumulator {
	return &StreamAccumulator{
		finalized: map[int]bool{},
	}
}

func (a *StreamAccumulator) Response() *MessagesResponse {
	out := a.resp
	out.Content = append([]ContentBlock{}, a.content...)
	return &out
}

func (a *StreamAccumulator) Consume(ev RawMessageStreamEvent) error {
	switch ev.Type {
	case "message_start":
		// { type, message: { id, role, ... } }
		if msg, ok := asMap(ev.Payload["message"]); ok {
			if id, ok := asString(msg["id"]); ok {
				a.resp.ID = id
			}
			if role, ok := asString(msg["role"]); ok {
				a.resp.Role = role
			}
		}
	case "content_block_start":
		// { type, index, content_block: {...} }
		idx, ok := asInt(ev.Payload["index"])
		if !ok {
			return fmt.Errorf("stream: content_block_start missing index")
		}
		cb, ok := asMap(ev.Payload["content_block"])
		if !ok {
			return fmt.Errorf("stream: content_block_start missing content_block")
		}
		typ, _ := asString(cb["type"])
		a.ensureIndex(idx)
		switch typ {
		case "text":
			a.content[idx] = ContentBlock{Type: "text", Text: ""}
		case "tool_use":
			id, _ := asString(cb["id"])
			name, _ := asString(cb["name"])
			// Tool input arrives as input_json_delta partial_json; we accumulate as string and parse on stop.
			a.content[idx] = ContentBlock{Type: "tool_use", ID: id, Name: name, Input: ""}
		default:
			// Keep an immutable copy of unknown blocks; QueryEngine will ignore them for now.
			id, _ := asString(cb["id"])
			a.content[idx] = ContentBlock{Type: typ, ID: id}
		}
	case "content_block_delta":
		idx, ok := asInt(ev.Payload["index"])
		if !ok {
			return fmt.Errorf("stream: content_block_delta missing index")
		}
		delta, ok := asMap(ev.Payload["delta"])
		if !ok {
			return fmt.Errorf("stream: content_block_delta missing delta")
		}
		dt, _ := asString(delta["type"])
		a.ensureIndex(idx)
		switch dt {
		case "text_delta":
			txt, _ := asString(delta["text"])
			if a.content[idx].Type == "" {
				a.content[idx] = ContentBlock{Type: "text", Text: ""}
			}
			a.content[idx].Text += txt
		case "input_json_delta":
			pj, _ := asString(delta["partial_json"])
			if a.content[idx].Type == "" {
				a.content[idx] = ContentBlock{Type: "tool_use", Input: ""}
			}
			if s, ok := a.content[idx].Input.(string); ok {
				a.content[idx].Input = s + pj
			} else {
				// Be resilient: if something else wrote Input, stringify by re-marshal.
				b, _ := json.Marshal(a.content[idx].Input)
				a.content[idx].Input = string(b) + pj
			}
		default:
			// citations_delta/thinking_delta/etc: ignore for phase-1.
		}
	case "content_block_stop":
		idx, ok := asInt(ev.Payload["index"])
		if !ok {
			return fmt.Errorf("stream: content_block_stop missing index")
		}
		a.ensureIndex(idx)
		if a.finalized[idx] {
			return nil
		}
		a.finalized[idx] = true

		// Finalize tool_use input: parse JSON if possible.
		if a.content[idx].Type == "tool_use" {
			if s, ok := a.content[idx].Input.(string); ok && s != "" {
				var v any
				if err := json.Unmarshal([]byte(s), &v); err == nil {
					a.content[idx].Input = v
				}
			}
		}
	case "message_delta":
		// { type, delta: { stop_reason, ... } }
		if d, ok := asMap(ev.Payload["delta"]); ok {
			if sr, ok := asString(d["stop_reason"]); ok {
				a.resp.StopReason = sr
			}
		}
	case "message_stop":
		// end
	default:
		// ignore
	}
	return nil
}

func (a *StreamAccumulator) ensureIndex(i int) {
	for len(a.content) <= i {
		a.content = append(a.content, ContentBlock{})
	}
}

func asMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func asString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

func asInt(v any) (int, bool) {
	switch n := v.(type) {
	case float64:
		return int(n), true
	case int:
		return n, true
	default:
		return 0, false
	}
}

