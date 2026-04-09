package memdir

import (
	"os"
	"path/filepath"
	"strings"
)

// Go analog of TS buildSearchingPastContextSection().
// Gate via env rather than Growthbook feature flag.
func buildSearchingPastContextSection(autoMemDir string) []string {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("CLAUDE_GO_SEARCH_PAST_CONTEXT")))
	if !(v == "1" || v == "true" || v == "yes" || v == "on") {
		return nil
	}
	projectDir, _ := os.Getwd()

	memSearch := `Grep with pattern="<search term>" path="` + autoMemDir + `" glob="*.md"`
	transcriptDir := filepath.Join(projectDir, ".claude-go", "messages-dump")
	transcriptSearch := `Grep with pattern="<search term>" path="` + transcriptDir + `" glob="*.json"`
	return []string{
		"## Searching past context",
		"",
		"When looking for past context:",
		"1. Search topic files in your memory directory:",
		"```",
		memSearch,
		"```",
		"2. Session transcript logs (last resort — large files, slow):",
		"```",
		transcriptSearch,
		"```",
		"Use narrow search terms (error messages, file paths, function names) rather than broad keywords.",
		"",
	}
}

