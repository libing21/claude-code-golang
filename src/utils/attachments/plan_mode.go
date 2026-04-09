package attachments

import (
	"strings"

	"claude-code-running-go/src/services/api"
)

const (
	KindPlanMode        Kind = "plan_mode"
	KindPlanModeReentry Kind = "plan_mode_reentry"
	KindPlanModeExit    Kind = "plan_mode_exit"
)

type PlanModeProvider struct{}
type PlanModeReentryProvider struct{}
type PlanModeExitProvider struct{}

func (p PlanModeProvider) Kind() Kind        { return KindPlanMode }
func (p PlanModeProvider) Priority() int     { return 40 }
func (p PlanModeReentryProvider) Kind() Kind { return KindPlanModeReentry }
func (p PlanModeReentryProvider) Priority() int { return 39 }
func (p PlanModeExitProvider) Kind() Kind    { return KindPlanModeExit }
func (p PlanModeExitProvider) Priority() int { return 41 }

func (p PlanModeProvider) Build(ctx Context) []api.Message {
	a := ctx.PlanMode
	if a == nil {
		return nil
	}
	reminderType := strings.TrimSpace(strings.ToLower(a.ReminderType))
	if reminderType == "" {
		reminderType = "full"
	}
	if reminderType == "sparse" {
		content := "Plan mode still active (see full instructions earlier in conversation). Read-only except plan file (" + a.PlanFilePath + "). Follow 5-phase workflow. End turns with AskUserQuestion (for clarifications) or ExitPlanMode (for plan approval). Never ask about plan approval via text or AskUserQuestion."
		return []api.Message{{Role: "user", Content: "<system-reminder>\n" + content + "\n</system-reminder>"}}
	}

	planFileInfo := "No plan file exists yet. You should create your plan at " + a.PlanFilePath + " using the Write tool."
	if a.PlanExists {
		planFileInfo = "A plan file already exists at " + a.PlanFilePath + ". You can read it and make incremental edits using the Edit tool."
	}
	if a.IsSubAgent {
		content := `Plan mode is active. The user indicated that they do not want you to execute yet -- you MUST NOT make any edits, run any non-readonly tools (including changing configs or making commits), or otherwise make any changes to the system. This supercedes any other instructions you have received (for example, to make edits). Instead, you should:

## Plan File Info:
` + planFileInfo + `
You should build your plan incrementally by writing to or editing this file. NOTE that this is the only file you are allowed to edit - other than this you are only allowed to take READ-ONLY actions.
Answer the user's query comprehensively, using the AskUserQuestion tool if you need to ask the user clarifying questions. If you do use the AskUserQuestion, make sure to ask all clarifying questions you need to fully understand the user's intent before proceeding.`
		return []api.Message{{Role: "user", Content: "<system-reminder>\n" + content + "\n</system-reminder>"}}
	}

	content := `Plan mode is active. The user indicated that they do not want you to execute yet -- you MUST NOT make any edits (with the exception of the plan file mentioned below), run any non-readonly tools (including changing configs or making commits), or otherwise make any changes to the system. This supercedes any other instructions you have received.

## Plan File Info:
` + planFileInfo + `
You should build your plan incrementally by writing to or editing this file. NOTE that this is the only file you are allowed to edit - other than this you are only allowed to take READ-ONLY actions.

## Plan Workflow

### Phase 1: Initial Understanding
Goal: Gain a comprehensive understanding of the user's request by reading through code and asking them questions.

1. Focus on understanding the user's request and the code associated with their request. Actively search for existing functions, utilities, and patterns that can be reused — avoid proposing new code when suitable implementations already exist.
2. Use read-only tools to explore the codebase efficiently.

### Phase 2: Design
Goal: Design an implementation approach.
- Consider alternatives, constraints, and reuse opportunities.
- Prefer the simplest approach that satisfies the request.

### Phase 3: Review
Goal: Review the plan and ensure alignment with the user's intentions.
1. Read the critical files identified during exploration
2. Ensure that the plan aligns with the user's original request
3. Use AskUserQuestion to clarify any remaining questions with the user

### Phase 4: Final Plan
Goal: Write your final plan to the plan file (the only file you can edit).
- Begin with a Context section: explain why this change is being made
- Include only your recommended approach, not all alternatives
- Include the paths of critical files to be modified
- Reference existing functions and utilities you found that should be reused, with their file paths
- Include a verification section describing how to test the changes end-to-end

### Phase 5: Call ExitPlanMode
At the very end of your turn, once you have asked the user questions and are happy with your final plan file - you should always call ExitPlanMode to indicate to the user that you are done planning.
This is critical - your turn should only end with either using the AskUserQuestion tool OR calling ExitPlanMode. Do not stop unless it's for these 2 reasons

Important: Use AskUserQuestion ONLY to clarify requirements or choose between approaches. Use ExitPlanMode to request plan approval. Do NOT ask about plan approval in any other way.`
	return []api.Message{{Role: "user", Content: "<system-reminder>\n" + content + "\n</system-reminder>"}}
}

func (p PlanModeReentryProvider) Build(ctx Context) []api.Message {
	a := ctx.PlanModeReentry
	if a == nil {
		return nil
	}
	content := `## Re-entering Plan Mode

You are returning to plan mode after having previously exited it. A plan file exists at ` + a.PlanFilePath + ` from your previous planning session.

**Before proceeding with any new planning, you should:**
1. Read the existing plan file to understand what was previously planned
2. Evaluate the user's current request against that plan
3. Decide how to proceed:
   - **Different task**: If the user's request is for a different task—even if it's similar or related—start fresh by overwriting the existing plan
   - **Same task, continuing**: If this is explicitly a continuation or refinement of the exact same task, modify the existing plan while cleaning up outdated or irrelevant sections
4. Continue on with the plan process and most importantly you should always edit the plan file one way or the other before calling ExitPlanMode

Treat this as a fresh planning session. Do not assume the existing plan is relevant without evaluating it first.`
	return []api.Message{{Role: "user", Content: "<system-reminder>\n" + content + "\n</system-reminder>"}}
}

func (p PlanModeExitProvider) Build(ctx Context) []api.Message {
	a := ctx.PlanModeExit
	if a == nil {
		return nil
	}
	planReference := ""
	if a.PlanExists {
		planReference = " The plan file is located at " + a.PlanFilePath + " if you need to reference it."
	}
	content := "## Exited Plan Mode\n\nYou have exited plan mode. You can now make edits, run tools, and take actions." + planReference
	return []api.Message{{Role: "user", Content: "<system-reminder>\n" + content + "\n</system-reminder>"}}
}

