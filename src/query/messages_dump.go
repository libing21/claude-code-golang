package query

import (
	"encoding/json"
	"os"
	"path/filepath"

	"claude-code-running-go/src/memdir"
	"claude-code-running-go/src/services/api"
	toolruntime "claude-code-running-go/src/services/tools"
)

func dumpTurn(dir string, step int, systemPrompt []string, tools []api.ToolSchema, msgs []api.Message, resp *api.MessagesResponse) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	prefix := filepath.Join(dir, "step-"+itoa(step))

	writeJSON(prefix+"-system.json", systemPrompt)
	writeJSON(prefix+"-tools.json", tools)
	writeJSON(prefix+"-messages.json", msgs)
	writeJSON(prefix+"-response.json", resp)
	// Memory debug: entrypoint truncation and paths used to build the memory section.
	if md := memdir.GetLastMemoryDebug(); md != nil {
		writeJSON(prefix+"-memory.json", md)
	}
	// Relevant memory debug: selection candidates/selected.
	if rd := memdir.GetLastRelevantDebug(); rd != nil {
		writeJSON(prefix+"-relevant-memories.json", rd)
	}
	// Relevant memory debug: extracted content, header, truncation notes.
	if ed := memdir.GetLastExtractedDebug(); ed != nil {
		writeJSON(prefix+"-relevant-memories-extracted.json", ed)
	}
	return nil
}

func dumpToolProgress(dir string, step int, tuc *toolruntime.ToolUseContext) {
	if tuc == nil {
		return
	}
	prefix := filepath.Join(dir, "step-"+itoa(step))
	writeJSON(prefix+"-tool-progress.json", map[string]any{
		"abortReason":                tuc.AbortReason(),
		"hasInterruptibleInProgress": tuc.HasInterruptibleToolInProgress(),
		"events":                     tuc.ProgressEvents(),
	})
}

func dumpStreamEvents(dir string, step int, events []api.RawMessageStreamEvent) {
	if len(events) == 0 {
		return
	}
	prefix := filepath.Join(dir, "step-"+itoa(step))
	writeJSON(prefix+"-stream-events.json", events)
}

func writeJSON(path string, v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, b, 0o644)
}

func itoa(i int) string {
	// Minimal int to string without fmt.
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var buf [32]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + (i % 10))
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
