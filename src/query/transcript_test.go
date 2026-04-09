package query

import (
	"path/filepath"
	"testing"

	"claude-code-running-go/src/services/api"
)

func TestFileTranscriptStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	store := NewFileTranscriptStore(filepath.Join(dir, "transcript.jsonl"))

	in := []api.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: []api.ContentBlock{{Type: "text", Text: "hi"}}},
		{Role: "user", Content: []map[string]any{
			{"type": "tool_result", "tool_use_id": "t1", "content": "ok"},
		}},
	}
	if err := store.Append(in); err != nil {
		t.Fatalf("append: %v", err)
	}
	got, err := store.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(got) != len(in) {
		t.Fatalf("message count mismatch: got=%d want=%d", len(got), len(in))
	}
}

func TestReconstructToolBudgetState(t *testing.T) {
	msgs := []api.Message{
		{
			Role: "user",
			Content: []map[string]any{
				{
					"type":        "tool_result",
					"tool_use_id": "a1",
					"content":     "plain content",
				},
				{
					"type":        "tool_result",
					"tool_use_id": "a2",
					"content":     "abc\n\n(Result truncated. Full content saved to /tmp/a2.txt)",
				},
			},
		},
	}
	st := ReconstructToolBudgetState(msgs)
	if _, ok := st.SeenIDs["a1"]; !ok {
		t.Fatalf("expected a1 in seen ids")
	}
	if _, ok := st.SeenIDs["a2"]; !ok {
		t.Fatalf("expected a2 in seen ids")
	}
	if _, ok := st.Replacements["a2"]; !ok {
		t.Fatalf("expected a2 replacement to be reconstructed")
	}
	if _, ok := st.Replacements["a1"]; ok {
		t.Fatalf("did not expect a1 replacement")
	}
}

