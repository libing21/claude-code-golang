package query

import (
	"path/filepath"
	"strings"

	"claude-code-running-go/src/services/api"
)

// scanAlreadySurfacedMemoryPaths tries to identify memory topic file paths that have already
// been injected via relevant_memories attachments, so we don't repeatedly re-inject them.
func scanAlreadySurfacedMemoryPaths(msgs []api.Message) map[string]struct{} {
	out := map[string]struct{}{}
	for _, m := range msgs {
		if m.Role != "user" {
			continue
		}
		s, ok := m.Content.(string)
		if !ok {
			continue
		}
		// Fast path: only relevant-memories attachments use this wrapper.
		if !strings.Contains(s, "<system-reminder>") || !strings.Contains(s, "Memory") {
			continue
		}
		for _, line := range strings.Split(s, "\n") {
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "Memory") {
				continue
			}
			// Examples:
			// "Memory: /abs/path/file.md:"
			// "Memory (saved 2 days ago): /abs/path/file.md:"
			idx := strings.Index(line, ": ")
			if idx < 0 {
				continue
			}
			rest := strings.TrimSpace(line[idx+2:])
			if !strings.HasSuffix(rest, ":") {
				continue
			}
			p := strings.TrimSpace(strings.TrimSuffix(rest, ":"))
			if p == "" {
				continue
			}
			// We only de-dupe absolute paths (topic file paths).
			if filepath.IsAbs(p) {
				out[p] = struct{}{}
			}
		}
	}
	return out
}

