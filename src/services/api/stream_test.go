package api

import (
	"context"
	"strings"
	"testing"
)

func TestReadSSE_ParsesEvents(t *testing.T) {
	src := strings.Join([]string{
		"event: message_start",
		"data: {\"type\":\"message_start\",\"message\":{\"id\":\"m1\"}}",
		"",
		"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hi\"}}",
		"",
		// Multi-line data should be joined with '\n'
		"data: {\"type\":\"message_delta\",",
		"data:  \"delta\":{\"stop_reason\":\"tool_use\"}}",
		"",
	}, "\n")

	var got []string
	err := readSSE(context.Background(), strings.NewReader(src), func(eventName string, data []byte) error {
		var ev RawMessageStreamEvent
		if err := ev.UnmarshalJSON(data); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		got = append(got, eventName+"|"+ev.Type)
		return nil
	})
	if err != nil {
		t.Fatalf("readSSE: %v", err)
	}
	want := []string{
		"message_start|message_start",
		"|content_block_delta",
		"|message_delta",
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("events mismatch\ngot:  %v\nwant: %v", got, want)
	}
}

func TestReadSSE_DoneSentinelStops(t *testing.T) {
	src := "data: [DONE]\n\n"
	count := 0
	err := readSSE(context.Background(), strings.NewReader(src), func(eventName string, data []byte) error {
		count++
		return nil
	})
	if err != nil {
		t.Fatalf("readSSE: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0 events, got %d", count)
	}
}

