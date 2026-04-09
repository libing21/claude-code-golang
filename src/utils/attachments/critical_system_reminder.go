package attachments

import (
	"strings"

	"claude-code-running-go/src/services/api"
)

const KindCriticalSystemReminder Kind = "critical_system_reminder"

type CriticalSystemReminderProvider struct{}

func (p CriticalSystemReminderProvider) Kind() Kind    { return KindCriticalSystemReminder }
func (p CriticalSystemReminderProvider) Priority() int { return 20 }

func (p CriticalSystemReminderProvider) Build(ctx Context) []api.Message {
	content := strings.TrimSpace(ctx.CriticalSystemReminder)
	if content == "" {
		return nil
	}
	return []api.Message{{Role: "user", Content: "<system-reminder>\n" + content + "\n</system-reminder>"}}
}

