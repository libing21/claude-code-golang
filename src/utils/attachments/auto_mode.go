package attachments

import "claude-code-running-go/src/services/api"

const (
	KindAutoMode     Kind = "auto_mode"
	KindAutoModeExit Kind = "auto_mode_exit"
)

type AutoModeProvider struct{}
type AutoModeExitProvider struct{}

func (p AutoModeProvider) Kind() Kind    { return KindAutoMode }
func (p AutoModeProvider) Priority() int { return 42 }
func (p AutoModeExitProvider) Kind() Kind    { return KindAutoModeExit }
func (p AutoModeExitProvider) Priority() int { return 43 }

func (p AutoModeProvider) Build(ctx Context) []api.Message {
	a := ctx.AutoMode
	if a == nil {
		return nil
	}
	content := `## Auto Mode Active

Auto mode is active. The user chose continuous, autonomous execution. You should:

1. **Execute immediately** — Start implementing right away. Make reasonable assumptions and proceed on low-risk work.
2. **Minimize interruptions** — Prefer making reasonable assumptions over asking questions for routine decisions.
3. **Prefer action over planning** — Do not enter plan mode unless the user explicitly asks. When in doubt, start coding.
4. **Expect course corrections** — The user may provide suggestions or course corrections at any point; treat those as normal input.
5. **Do not take overly destructive actions** — Auto mode is not a license to destroy. Anything that deletes data or modifies shared or production systems still needs explicit user confirmation. If you reach such a decision point, ask and wait, or course correct to a safer method instead.
6. **Avoid data exfiltration** — Post even routine messages to chat platforms or work tickets only if the user has directed you to. You must not share secrets unless the user has explicitly authorized both that specific secret and its destination.`
	if a.ReminderType == "sparse" {
		content = `Auto mode still active (see full instructions earlier in conversation). Execute autonomously, minimize interruptions, prefer action over planning.`
	}
	return []api.Message{{Role: "user", Content: "<system-reminder>\n" + content + "\n</system-reminder>"}}
}

func (p AutoModeExitProvider) Build(ctx Context) []api.Message {
	if !ctx.AutoModeExit {
		return nil
	}
	content := `## Exited Auto Mode

You have exited auto mode. The user may now want to interact more directly. You should ask clarifying questions when the approach is ambiguous rather than making assumptions.`
	return []api.Message{{Role: "user", Content: "<system-reminder>\n" + content + "\n</system-reminder>"}}
}

