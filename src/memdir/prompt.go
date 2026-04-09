package memdir

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)
const (
	ENTRYPOINT_NAME      = "MEMORY.md"
	MAX_ENTRYPOINT_LINES = 200
	MAX_ENTRYPOINT_BYTES = 25_000
)

type EntrypointTruncation struct {
	Content         string `json:"content"`
	LineCount       int    `json:"lineCount"`
	ByteCount       int    `json:"byteCount"`
	WasLineTruncated bool  `json:"wasLineTruncated"`
	WasByteTruncated bool  `json:"wasByteTruncated"`
}

type MemoryDebug struct {
	Cwd              string              `json:"cwd"`
	AutoDir          string              `json:"autoDir"`
	TeamDir          string              `json:"teamDir"`
	AutoEntrypoint   string              `json:"autoEntrypoint"`
	TeamEntrypoint   string              `json:"teamEntrypoint"`
	AutoTruncation   *EntrypointTruncation `json:"autoTruncation,omitempty"`
	TeamTruncation   *EntrypointTruncation `json:"teamTruncation,omitempty"`
	AutoEmpty        bool                `json:"autoEmpty"`
	TeamEmpty        bool                `json:"teamEmpty"`
}

var lastMemoryDebug *MemoryDebug

func GetLastMemoryDebug() *MemoryDebug { return lastMemoryDebug }

func formatFileSize(bytes int) string {
	// Close-enough analog of TS formatFileSize for warnings.
	const (
		KB = 1024
		MB = 1024 * KB
	)
	if bytes >= MB {
		return fmt.Sprintf("%.1fMB", float64(bytes)/float64(MB))
	}
	if bytes >= KB {
		return fmt.Sprintf("%.1fKB", float64(bytes)/float64(KB))
	}
	return fmt.Sprintf("%dB", bytes)
}

// TruncateEntrypointContent matches TS truncateEntrypointContent: line cap first, then byte cap.
func TruncateEntrypointContent(raw string) EntrypointTruncation {
	trimmed := strings.TrimSpace(raw)
	contentLines := []string{}
	if trimmed != "" {
		contentLines = strings.Split(trimmed, "\n")
	}
	lineCount := len(contentLines)
	byteCount := len(trimmed)

	wasLineTruncated := lineCount > MAX_ENTRYPOINT_LINES
	wasByteTruncated := byteCount > MAX_ENTRYPOINT_BYTES

	if !wasLineTruncated && !wasByteTruncated {
		return EntrypointTruncation{
			Content:          trimmed,
			LineCount:        lineCount,
			ByteCount:        byteCount,
			WasLineTruncated: wasLineTruncated,
			WasByteTruncated: wasByteTruncated,
		}
	}

	truncated := trimmed
	if wasLineTruncated {
		truncated = strings.Join(contentLines[:MAX_ENTRYPOINT_LINES], "\n")
	}
	if len(truncated) > MAX_ENTRYPOINT_BYTES {
		cutAt := strings.LastIndex(truncated[:MAX_ENTRYPOINT_BYTES], "\n")
		if cutAt > 0 {
			truncated = truncated[:cutAt]
		} else {
			truncated = truncated[:MAX_ENTRYPOINT_BYTES]
		}
	}

	reason := ""
	switch {
	case wasByteTruncated && !wasLineTruncated:
		reason = fmt.Sprintf("%s (limit: %s) — index entries are too long", formatFileSize(byteCount), formatFileSize(MAX_ENTRYPOINT_BYTES))
	case wasLineTruncated && !wasByteTruncated:
		reason = fmt.Sprintf("%d lines (limit: %d)", lineCount, MAX_ENTRYPOINT_LINES)
	default:
		reason = fmt.Sprintf("%d lines and %s", lineCount, formatFileSize(byteCount))
	}

	truncated = truncated + fmt.Sprintf("\n\n> WARNING: %s is %s. Only part of it was loaded. Keep index entries to one line under ~200 chars; move detail into topic files.", ENTRYPOINT_NAME, reason)
	return EntrypointTruncation{
		Content:          truncated,
		LineCount:        lineCount,
		ByteCount:        byteCount,
		WasLineTruncated: wasLineTruncated,
		WasByteTruncated: wasByteTruncated,
	}
}

func readFileIfExists(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

// BuildMemoryPrompt is a simplified Go analog of TS loadMemoryPrompt/buildCombinedMemoryPrompt,
// but includes the contents of both private and team MEMORY.md entrypoints (truncated).
func BuildMemoryPrompt(cwd string) string {
	if !IsAutoMemoryEnabled() {
		return ""
	}

	autoDir := GetAutoMemPath(cwd)
	teamDir := GetTeamMemPath(cwd)
	_ = os.MkdirAll(autoDir, 0o755)
	_ = os.MkdirAll(teamDir, 0o755)
	// Team sync is a TS feature; Go port currently treats it as best-effort.
	_ = SyncTeamMemOnce(cwd, teamDir)
	autoEntrypoint := filepath.Join(autoDir, ENTRYPOINT_NAME)
	teamEntrypoint := filepath.Join(teamDir, ENTRYPOINT_NAME)

	autoRaw := readFileIfExists(autoEntrypoint)
	teamRaw := readFileIfExists(teamEntrypoint)
	autoEmpty := strings.TrimSpace(autoRaw) == ""
	teamEmpty := strings.TrimSpace(teamRaw) == ""

	var autoTrunc *EntrypointTruncation
	if !autoEmpty {
		t := TruncateEntrypointContent(autoRaw)
		autoTrunc = &t
	}
	var teamTrunc *EntrypointTruncation
	if !teamEmpty {
		t := TruncateEntrypointContent(teamRaw)
		teamTrunc = &t
	}

	lastMemoryDebug = &MemoryDebug{
		Cwd:            cwd,
		AutoDir:        autoDir,
		TeamDir:        teamDir,
		AutoEntrypoint: autoEntrypoint,
		TeamEntrypoint: teamEntrypoint,
		AutoTruncation: autoTrunc,
		TeamTruncation: teamTrunc,
		AutoEmpty:      autoEmpty,
		TeamEmpty:      teamEmpty,
	}
	lines := []string{
		"# Memory",
		"",
		fmt.Sprintf("You have a persistent, file-based memory system with two directories: a private directory at `%s` and a shared team directory at `%s`.", autoDir, teamDir),
		"Both directories already exist — write to them directly with the Write tool (do not run mkdir or check for their existence).",
		"",
		"If the user explicitly asks you to remember something, save it immediately. If they ask you to forget something, find and remove the relevant entry.",
		"",
		"## Entrypoints (indexes)",
		"",
		fmt.Sprintf("Each directory has its own `%s` index. Both indexes are loaded into your conversation context. Lines after %d will be truncated, and very large files may be byte-truncated — keep entries concise.", ENTRYPOINT_NAME, MAX_ENTRYPOINT_LINES),
		fmt.Sprintf("`%s` is an index, not a memory: each entry should be one line under ~150 characters: `- [Title](file.md) — one-line hook`. It has no frontmatter. Never write memory content directly into `%s`.", ENTRYPOINT_NAME, ENTRYPOINT_NAME),
		"",
		"## How to save memories",
		"",
		"Saving a memory is a two-step process:",
		"",
		"**Step 1** — write the memory to its own file in the chosen directory (private or team) using this frontmatter format:",
		"",
		"```yaml",
		"---",
		`name: "Title"`,
		`description: "One sentence summary"`,
		`type: "user|feedback|project|reference"`,
		"---",
		"```",
		"",
		fmt.Sprintf("**Step 2** — add a pointer to that file in the same directory's `%s` index.", ENTRYPOINT_NAME),
		"",
		"## Scope",
		"- private: memories that are private between you and the current user (stored at the root private directory).",
		"- team: memories shared within this project (stored under the team directory).",
		"- NEVER save secrets (API keys, credentials) into shared team memory.",
		"",
		"## When to access memories",
		"- When memories seem relevant or the user references prior work.",
		"- You MUST access memory when the user explicitly asks you to check, recall, or remember.",
		"- If the user says to ignore memory: proceed as if MEMORY.md were empty.",
		"",
		"## Persistence guidance",
		"- Prefer plans (not memory) for tracking the approach within the current task.",
		"- Prefer tasks/todos (not memory) for tracking current conversation progress.",
		"",
		"## MEMORY.md (private)",
		"",
	}

	if autoEmpty {
		lines = append(lines, fmt.Sprintf("Your %s is currently empty. When you save new memories, they will appear here.", ENTRYPOINT_NAME))
	} else {
		lines = append(lines, autoTrunc.Content)
	}
	lines = append(lines, "", "## MEMORY.md (team)", "")
	if teamEmpty {
		lines = append(lines, fmt.Sprintf("Your %s is currently empty. When you save new memories, they will appear here.", ENTRYPOINT_NAME))
	} else {
		lines = append(lines, teamTrunc.Content)
	}

	if sec := buildSearchingPastContextSection(autoDir); len(sec) > 0 {
		lines = append(lines, "")
		lines = append(lines, sec...)
	}
	return strings.Join(lines, "\n")
}
