package query

import (
	"context"
	"os"
	"strings"

	"claude-code-running-go/src/services/api"
	"claude-code-running-go/src/utils/attachments"
)

type Output struct {
	Text string
}

func envTruthy(key string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func planFilePathForSession() string {
	if p := strings.TrimSpace(os.Getenv("CLAUDE_GO_PLAN_FILE")); p != "" {
		return p
	}
	cwd, _ := os.Getwd()
	return cwd + string(os.PathSeparator) + ".claude-go" + string(os.PathSeparator) + "plan.md"
}

func inferPlanModeAttachment(permissionMode string) *attachments.PlanModeAttachment {
	mode := strings.TrimSpace(strings.ToLower(permissionMode))
	if mode != "plan" && !envTruthy("CLAUDE_GO_PLAN_MODE") {
		return nil
	}
	p := planFilePathForSession()
	_, err := os.Stat(p)
	exists := err == nil
	reminder := strings.TrimSpace(strings.ToLower(os.Getenv("CLAUDE_GO_PLAN_MODE_REMINDER")))
	if reminder == "" {
		reminder = "full"
	}
	return &attachments.PlanModeAttachment{
		ReminderType: reminder,
		IsSubAgent:   envTruthy("CLAUDE_GO_PLAN_MODE_SUBAGENT"),
		PlanFilePath: p,
		PlanExists:   exists,
	}
}

func inferPlanModeReentryAttachment() *attachments.PlanModeReentryAttachment {
	if !envTruthy("CLAUDE_GO_PLAN_MODE_REENTRY") {
		return nil
	}
	return &attachments.PlanModeReentryAttachment{PlanFilePath: planFilePathForSession()}
}

func inferPlanModeExitAttachment() *attachments.PlanModeExitAttachment {
	if !envTruthy("CLAUDE_GO_PLAN_MODE_EXIT") {
		return nil
	}
	p := planFilePathForSession()
	_, err := os.Stat(p)
	return &attachments.PlanModeExitAttachment{PlanFilePath: p, PlanExists: err == nil}
}

func inferAutoModeAttachment(permissionMode string) *attachments.AutoModeAttachment {
	mode := strings.TrimSpace(strings.ToLower(permissionMode))
	if mode != "auto" && !envTruthy("CLAUDE_GO_AUTO_MODE") {
		return nil
	}
	reminder := strings.TrimSpace(strings.ToLower(os.Getenv("CLAUDE_GO_AUTO_MODE_REMINDER")))
	if reminder == "" {
		reminder = "full"
	}
	return &attachments.AutoModeAttachment{ReminderType: reminder}
}

func isHumanTurnMessage(msg api.Message) bool {
	if msg.Role != "user" {
		return false
	}
	switch v := msg.Content.(type) {
	case string:
		return !strings.Contains(v, "<system-reminder>")
	default:
		// tool_result and block content are not human turns
		return false
	}
}

func isAttachmentStringMessage(msg api.Message, needle string) bool {
	if msg.Role != "user" {
		return false
	}
	s, ok := msg.Content.(string)
	if !ok {
		return false
	}
	return strings.Contains(s, needle)
}

const attachmentTurnsBetween = 5
const attachmentFullEvery = 5

func inferPlanModeAttachmentFromMessages(permissionMode string, msgs []api.Message) *attachments.PlanModeAttachment {
	mode := strings.TrimSpace(strings.ToLower(permissionMode))
	if mode != "plan" && !envTruthy("CLAUDE_GO_PLAN_MODE") {
		return nil
	}
	turnsSinceLast := 0
	foundPlan := false
	countSinceExit := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if isHumanTurnMessage(m) {
			turnsSinceLast++
			continue
		}
		if isAttachmentStringMessage(m, "## Exited Plan Mode") {
			break
		}
		if isAttachmentStringMessage(m, "Plan mode is active.") || isAttachmentStringMessage(m, "Plan mode still active") {
			foundPlan = true
			countSinceExit++
			if turnsSinceLast == 0 {
				// keep scanning for count
			} else {
				break
			}
		}
	}
	if foundPlan && turnsSinceLast < attachmentTurnsBetween {
		return nil
	}
	attachmentCount := countSinceExit + 1
	reminder := "sparse"
	if attachmentCount%attachmentFullEvery == 1 {
		reminder = "full"
	}
	p := planFilePathForSession()
	_, err := os.Stat(p)
	return &attachments.PlanModeAttachment{
		ReminderType: reminder,
		IsSubAgent:   envTruthy("CLAUDE_GO_PLAN_MODE_SUBAGENT"),
		PlanFilePath: p,
		PlanExists:   err == nil,
	}
}

func inferAutoModeAttachmentFromMessages(permissionMode string, msgs []api.Message) *attachments.AutoModeAttachment {
	mode := strings.TrimSpace(strings.ToLower(permissionMode))
	if mode != "auto" && !envTruthy("CLAUDE_GO_AUTO_MODE") {
		return nil
	}
	turnsSinceLast := 0
	found := false
	countSinceExit := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if isHumanTurnMessage(m) {
			turnsSinceLast++
			continue
		}
		if isAttachmentStringMessage(m, "## Exited Auto Mode") {
			break
		}
		if isAttachmentStringMessage(m, "## Auto Mode Active") || isAttachmentStringMessage(m, "Auto mode still active") {
			found = true
			countSinceExit++
			if turnsSinceLast == 0 {
			} else {
				break
			}
		}
	}
	if found && turnsSinceLast < attachmentTurnsBetween {
		return nil
	}
	attachmentCount := countSinceExit + 1
	reminder := "sparse"
	if attachmentCount%attachmentFullEvery == 1 {
		reminder = "full"
	}
	return &attachments.AutoModeAttachment{ReminderType: reminder}
}

func parseCSVEnv(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func buildDeferredToolsDeltaFromEnv() *attachments.DeferredToolsDeltaAttachment {
	added := parseCSVEnv("CLAUDE_GO_DEFERRED_TOOLS")
	removed := parseCSVEnv("CLAUDE_GO_REMOVED_DEFERRED_TOOLS")
	if len(added) == 0 && len(removed) == 0 {
		return nil
	}
	return &attachments.DeferredToolsDeltaAttachment{AddedLines: added, RemovedNames: removed}
}

func buildAgentListingDeltaFromEnv() *attachments.AgentListingDeltaAttachment {
	addedTypes := parseCSVEnv("CLAUDE_GO_AGENT_TYPES")
	removedTypes := parseCSVEnv("CLAUDE_GO_REMOVED_AGENT_TYPES")
	if len(addedTypes) == 0 && len(removedTypes) == 0 {
		return nil
	}
	addedLines := make([]string, 0, len(addedTypes))
	for _, t := range addedTypes {
		addedLines = append(addedLines, "- "+t)
	}
	return &attachments.AgentListingDeltaAttachment{
		AddedTypes:          addedTypes,
		AddedLines:          addedLines,
		RemovedTypes:        removedTypes,
		IsInitial:           !envTruthy("CLAUDE_GO_AGENT_LISTING_NOT_INITIAL"),
		ShowConcurrencyNote: !envTruthy("CLAUDE_GO_SUBSCRIPTION_PRO"),
	}
}

// RunOnceWithToolLoop is the phase-1 entry: a single user prompt, running a minimal
// tool loop (Read/Glob/Grep). This is the main place you debug agent behavior.
// TS reference: src/query.ts + services/tools/*
func RunOnceWithToolLoop(
	ctx context.Context,
	client *api.Client,
	systemPrompt []string,
	userPrompt string,
	permissionMode string,
	allowedTools []string,
	disallowedTools []string,
	mcpConfigPath string,
	pluginDirs []string,
	skillDirs []string,
	debug bool,
	messagesDumpDir string,
) (Output, error) {
	engine := NewQueryEngine(QueryEngineConfig{
		Client:          client,
		SystemPrompt:    systemPrompt,
		PermissionMode:  permissionMode,
		AllowedTools:    allowedTools,
		DisallowedTools: disallowedTools,
		MCPConfigPath:   mcpConfigPath,
		PluginDirs:      pluginDirs,
		SkillDirs:       skillDirs,
		Debug:           debug,
		MessagesDumpDir: messagesDumpDir,
		// Use QueryEngine defaults unless the caller opts into overrides.
		// (Tests and extensions can pass MaxSteps/ModelOverride via QueryEngineConfig directly.)
	})
	return engine.RunOnce(ctx, userPrompt)
}
