package query

import (
	"testing"

	"claude-code-running-go/src/services/api"
)

func TestScanAlreadySurfacedMemoryPaths(t *testing.T) {
	msgs := []api.Message{
		{
			Role: "user",
			Content: "<system-reminder>\nMemory: /tmp/a.md:\n\nhello\n</system-reminder>",
		},
		{
			Role: "user",
			Content: "<system-reminder>\nMemory (saved 2 days ago): /tmp/b.md:\n\nworld\n</system-reminder>",
		},
	}
	got := scanAlreadySurfacedMemoryPaths(msgs)
	if _, ok := got["/tmp/a.md"]; !ok {
		t.Fatalf("expected /tmp/a.md")
	}
	if _, ok := got["/tmp/b.md"]; !ok {
		t.Fatalf("expected /tmp/b.md")
	}
}

