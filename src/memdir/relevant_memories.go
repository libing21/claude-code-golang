package memdir

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"claude-code-running-go/src/utils/attachments"

	"claude-code-running-go/src/services/api"
	"claude-code-running-go/src/utils/model"
)

// Port of TS src/memdir/findRelevantMemories.ts

type RelevantMemory struct {
	Path    string `json:"path"`
	MtimeMs int64  `json:"mtimeMs"`
}

const (
	maxMemoryLines = 200
	maxMemoryBytes = 4096
	maxSessionBytes = 60 * 1024
)

const selectMemoriesSystemPrompt = `You are selecting memories that will be useful to Claude Code as it processes a user's query. You will be given the user's query and a list of available memory files with their filenames and descriptions.

Return a list of filenames for the memories that will clearly be useful to Claude Code as it processes the user's query (up to 5). Only include memories that you are certain will be helpful based on their name and description.
- If you are unsure if a memory will be useful in processing the user's query, then do not include it in your list. Be selective and discerning.
- If there are no memories in the list that would clearly be useful, feel free to return an empty list.
- If a list of recently-used tools is provided, do not select memories that are usage reference or API documentation for those tools (Claude Code is already exercising them). DO still select memories containing warnings, gotchas, or known issues about those tools — active use is exactly when those matter.
`

type selectedMemoriesJSON struct {
	SelectedMemories []string `json:"selected_memories"`
}

type relevantDebug struct {
	Query           string        `json:"query"`
	Candidates      []MemoryHeader `json:"candidates"`
	Selected        []string      `json:"selected"`
	RecentTools     []string      `json:"recentTools"`
	SystemPrompt    string        `json:"systemPrompt"`
}

var lastRelevantDebug *relevantDebug

func GetLastRelevantDebug() any { return lastRelevantDebug }

type extractedDebug struct {
	Extracted []attachments.RelevantMemoryAttachment `json:"extracted"`
	TotalBytes int `json:"totalBytes"`
}

var lastExtractedDebug *extractedDebug

func GetLastExtractedDebug() any { return lastExtractedDebug }

func isRelevantMemoriesEnabled() bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv("CLAUDE_GO_RELEVANT_MEMORIES")))
	// Enabled by default unless explicitly disabled.
	return !(v == "0" || v == "false" || v == "off" || v == "no")
}

func memoryHeader(path string, mtimeMs int64) string {
	staleness := memoryFreshnessText(mtimeMs)
	if staleness != "" {
		return staleness + "\n\nMemory: " + path + ":"
	}
	return "Memory (saved " + memoryAge(mtimeMs) + "): " + path + ":"
}

func ExtractRelevantMemories(ctx context.Context, rels []RelevantMemory) ([]attachments.RelevantMemoryAttachment, error) {
	out := make([]attachments.RelevantMemoryAttachment, 0, len(rels))
	totalBytes := 0
	for _, r := range rels {
		res, err := readFileInRange(ctx, r.Path, maxMemoryLines, maxMemoryBytes, true)
		if err != nil {
			continue
		}
		truncated := res.TruncatedByBytes
		if truncated {
			limit := res.LineCount
			content := res.Content +
				fmt.Sprintf("\n\n> This memory file was truncated (%d byte limit). Use the Read tool to view the complete file at: %s", maxMemoryBytes, r.Path)
			out = append(out, attachments.RelevantMemoryAttachment{
				Path:    r.Path,
				Content: content,
				MtimeMs: r.MtimeMs,
				Header:  memoryHeader(r.Path, r.MtimeMs),
				Limit:   &limit,
			})
		} else {
			out = append(out, attachments.RelevantMemoryAttachment{
				Path:    r.Path,
				Content: res.Content,
				MtimeMs: r.MtimeMs,
				Header:  memoryHeader(r.Path, r.MtimeMs),
			})
		}
		totalBytes += len(res.Content)
		if totalBytes >= maxSessionBytes {
			break
		}
	}
	lastExtractedDebug = &extractedDebug{Extracted: out, TotalBytes: totalBytes}
	return out, nil
}

func FindRelevantMemories(ctx context.Context, client *api.Client, query string, memoryDir string, recentTools []string, alreadySurfaced map[string]struct{}) ([]RelevantMemory, error) {
	if !isRelevantMemoriesEnabled() {
		return nil, nil
	}
	// Single-word prompts lack enough context for meaningful term extraction (TS parity).
	if strings.TrimSpace(query) == "" || !strings.ContainsAny(strings.TrimSpace(query), " \t\n") {
		return nil, nil
	}

	memories, _ := scanMemoryFiles(ctx, memoryDir)
	filtered := make([]MemoryHeader, 0, len(memories))
	for _, m := range memories {
		if alreadySurfaced != nil {
			if _, ok := alreadySurfaced[m.FilePath]; ok {
				continue
			}
		}
		filtered = append(filtered, m)
	}
	if len(filtered) == 0 {
		lastRelevantDebug = &relevantDebug{Query: query, Candidates: nil, Selected: nil, RecentTools: recentTools, SystemPrompt: selectMemoriesSystemPrompt}
		return nil, nil
	}

	selected, err := selectRelevantMemories(ctx, client, query, filtered, recentTools)
	if err != nil {
		lastRelevantDebug = &relevantDebug{Query: query, Candidates: filtered, Selected: nil, RecentTools: recentTools, SystemPrompt: selectMemoriesSystemPrompt}
		return nil, nil
	}
	lastRelevantDebug = &relevantDebug{Query: query, Candidates: filtered, Selected: selected, RecentTools: recentTools, SystemPrompt: selectMemoriesSystemPrompt}

	byFilename := map[string]MemoryHeader{}
	for _, m := range filtered {
		byFilename[m.Filename] = m
	}
	out := make([]RelevantMemory, 0, 5)
	for _, fn := range selected {
		if m, ok := byFilename[fn]; ok {
			out = append(out, RelevantMemory{Path: m.FilePath, MtimeMs: m.MtimeMs})
		}
	}
	return out, nil
}

func selectRelevantMemories(ctx context.Context, client *api.Client, query string, memories []MemoryHeader, recentTools []string) ([]string, error) {
	valid := map[string]struct{}{}
	for _, m := range memories {
		valid[m.Filename] = struct{}{}
	}
	manifest := formatMemoryManifest(memories)
	toolsSection := ""
	if len(recentTools) > 0 {
		toolsSection = "\n\nRecently used tools: " + strings.Join(recentTools, ", ")
	}

	// Side-query: use default sonnet model for selector, max_tokens=256, json_schema output.
	selectorModel := model.GetDefaultSonnetModel()
	resp, err := client.CreateMessage(ctx, api.CreateMessageInput{
		SystemPrompt: []string{selectMemoriesSystemPrompt},
		Messages: []api.Message{{
			Role:    "user",
			Content: "Query: " + query + "\n\nAvailable memories:\n" + manifest + toolsSection,
		}},
		MaxTokens: 256,
		Model:     selectorModel,
		OutputFormat: map[string]any{
			"type": "json_schema",
			"schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"selected_memories": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"required":             []string{"selected_memories"},
				"additionalProperties": false,
			},
		},
	})
	if err != nil {
		return nil, err
	}
	var text string
	for _, b := range resp.Content {
		if b.Type == "text" {
			text = b.Text
			break
		}
	}
	if strings.TrimSpace(text) == "" {
		return nil, nil
	}
	var parsed selectedMemoriesJSON
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		// Some gateways wrap JSON in prose; best-effort extract.
		start := strings.Index(text, "{")
		end := strings.LastIndex(text, "}")
		if start >= 0 && end > start {
			_ = json.Unmarshal([]byte(text[start:end+1]), &parsed)
		}
	}
	out := make([]string, 0, len(parsed.SelectedMemories))
	for _, f := range parsed.SelectedMemories {
		if _, ok := valid[f]; ok {
			out = append(out, f)
		}
	}
	if len(out) > 5 {
		out = out[:5]
	}
	return out, nil
}
