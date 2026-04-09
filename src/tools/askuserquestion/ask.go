package askuserquestion

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"claude-code-running-go/src/tool"
)

type Input struct {
	Questions any `json:"questions"`
}

type AskUserQuestionTool struct{}

func New() *AskUserQuestionTool { return &AskUserQuestionTool{} }

func (t *AskUserQuestionTool) Name() string { return "AskUserQuestion" }

func (t *AskUserQuestionTool) Prompt() string {
	return strings.TrimSpace(`- Ask the user one or more clarifying questions.
- In print/non-interactive mode this tool cannot be used.`)
}

func (t *AskUserQuestionTool) InputSchema() json.RawMessage {
	// Keep schema permissive for now; TS uses a rich nested schema.
	return json.RawMessage(`{
  "type":"object",
  "additionalProperties":false,
  "properties":{
    "questions":{"type":"array","description":"Questions to ask the user"}
  },
  "required":["questions"]
}`)
}

func (t *AskUserQuestionTool) IsReadOnly(_ any) bool        { return true }
func (t *AskUserQuestionTool) IsConcurrencySafe(_ any) bool { return true }

func (t *AskUserQuestionTool) ValidateInput(input any) error {
	_ = input
	return nil
}

func (t *AskUserQuestionTool) CheckPermissions(_ context.Context, _ any, _ tool.PermissionContext) (tool.PermissionDecision, any, error) {
	return tool.PermissionDecision{Behavior: tool.PermissionBehaviorAsk, Reason: "requires interactive UI"}, nil, nil
}

func (t *AskUserQuestionTool) Call(_ context.Context, _ any) (tool.ToolResult, error) {
	return tool.ToolResult{IsError: true, Content: "AskUserQuestion requires interactive UI; not supported in Go print mode"}, fmt.Errorf("interactive tool not supported")
}

