package attachments

import (
	"strings"

	"claude-code-running-go/src/services/api"
)

const KindDateChange Kind = "date_change"

type DateChangeProvider struct{}

func (p DateChangeProvider) Kind() Kind    { return KindDateChange }
func (p DateChangeProvider) Priority() int { return 10 }

func (p DateChangeProvider) Build(ctx Context) []api.Message {
	if !ctx.DateChanged || strings.TrimSpace(ctx.NewDate) == "" {
		return nil
	}
	content := "<system-reminder>\n" +
		"The date has changed. Today's date is now " + strings.TrimSpace(ctx.NewDate) + ". DO NOT mention this to the user explicitly because they are already aware." +
		"\n</system-reminder>"
	return []api.Message{{Role: "user", Content: content}}
}

