package attachments

import (
	"strings"

	"claude-code-running-go/src/services/api"
)

const KindOutputStyle Kind = "output_style"

type OutputStyleProvider struct{}

func (p OutputStyleProvider) Kind() Kind    { return KindOutputStyle }
func (p OutputStyleProvider) Priority() int { return 30 }

func (p OutputStyleProvider) Build(ctx Context) []api.Message {
	name := strings.TrimSpace(ctx.OutputStyleName)
	if name == "" || strings.EqualFold(name, "default") {
		return nil
	}
	content := "<system-reminder>\n" +
		name + " output style is active. Remember to follow the specific guidelines for this style." +
		"\n</system-reminder>"
	return []api.Message{{Role: "user", Content: content}}
}

