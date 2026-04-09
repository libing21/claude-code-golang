package attachments

import (
	"strings"

	"claude-code-running-go/src/services/api"
)

const KindRelevantMemories Kind = "relevant_memories"

type RelevantMemoriesProvider struct{}

func (p RelevantMemoriesProvider) Kind() Kind    { return KindRelevantMemories }
func (p RelevantMemoriesProvider) Priority() int { return 50 }

func (p RelevantMemoriesProvider) Build(ctx Context) []api.Message {
	if len(ctx.RelevantMemories) == 0 {
		return nil
	}
	out := make([]api.Message, 0, len(ctx.RelevantMemories))
	for _, m := range ctx.RelevantMemories {
		header := strings.TrimSpace(m.Header)
		if header == "" {
			header = "Memory: " + m.Path + ":"
		}
		body := header + "\n\n" + strings.TrimSpace(m.Content)
		content := "<system-reminder>\n" + body + "\n</system-reminder>"
		out = append(out, api.Message{Role: "user", Content: content})
	}
	return out
}

