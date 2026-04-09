package bash

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"claude-code-running-go/src/tool"
	"claude-code-running-go/src/utils/permissions"
)

type Input struct {
	Command        string `json:"command"`
	Cwd            string `json:"cwd,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
}

type BashTool struct{}

func New() *BashTool { return &BashTool{} }

func (t *BashTool) Name() string { return "Bash" }

func (t *BashTool) Prompt() string {
	// Ported (condensed) from TS BashTool/prompt.ts. Keep behavior guidance close to original.
	return strings.TrimSpace(`# Bash tool

Use this tool to run shell commands locally when a dedicated tool is not suitable.

IMPORTANT safety rules:
- Prefer dedicated tools over Bash: use Read/Glob/Grep/Edit/Write when applicable.
- NEVER run destructive git commands (push --force, reset --hard, checkout ., restore ., clean -f, branch -D) unless explicitly requested by the user.
- NEVER skip hooks (--no-verify, --no-gpg-sign, etc) unless explicitly requested.
- NEVER run interactive git commands (-i) such as git rebase -i or git add -i.
- Treat high-risk commands (rm -rf, sudo, chmod -R, chown -R) as requiring explicit user confirmation.

Git workflow expectations (when asked to commit/PR):
- Only create commits when explicitly requested.
- Prefer staging specific files, avoid git add -A / git add . unless requested.
- After hook failure, fix and create a NEW commit; do not amend unless requested.
- Do not push unless requested.`)
}

func (t *BashTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type":"object",
  "additionalProperties":false,
  "properties":{
    "command":{"type":"string","description":"Shell command to execute"},
    "cwd":{"type":"string","description":"Working directory (optional)"},
    "timeout_seconds":{"type":"integer","minimum":1,"description":"Timeout in seconds (optional)"}
  },
  "required":["command"]
}`)
}

func (t *BashTool) IsReadOnly(_ any) bool        { return false }
func (t *BashTool) IsConcurrencySafe(_ any) bool { return false }
func (t *BashTool) InterruptBehavior(_ any) tool.InterruptBehavior {
	// TS: Bash is generally cancelable on user interrupt.
	return tool.InterruptBehaviorCancel
}

func (t *BashTool) ValidateInput(input any) error {
	switch v := input.(type) {
	case Input:
		if strings.TrimSpace(v.Command) == "" {
			return fmt.Errorf("command is required")
		}
	}
	return nil
}

func (t *BashTool) CheckPermissions(_ context.Context, input any, _ tool.PermissionContext) (tool.PermissionDecision, any, error) {
	// Apply TS-like safety checks. These should not be bypassed silently.
	cmd := extractCommandString(input)
	n := normalizeCommand(cmd)
	if n == "" {
		return tool.PermissionDecision{Behavior: tool.PermissionBehaviorDeny, Reason: "empty command"}, nil, nil
	}

	// Validate working directory if provided.
	if cwd := extractCwdString(input); strings.TrimSpace(cwd) != "" {
		dec, abs := permissions.CheckPath(permissions.DefaultPathPolicy(), permissions.OpBash, cwd)
		if dec.Behavior != tool.PermissionBehaviorAllow {
			// Keep the decision as-is; it already contains CheckID/Tag.
			return dec, nil, nil
		}
		// Normalize cwd for the actual call.
		switch v := input.(type) {
		case Input:
			v.Cwd = abs
			input = v
		case map[string]any:
			v["cwd"] = abs
			input = v
		}
	}

	// Catch command substitution / expansion patterns that can bypass naive policy.
	if reason := DangerousSubstitutionReason(n); reason != "" {
		return tool.PermissionDecision{Behavior: tool.PermissionBehaviorAsk, Reason: "dangerous shell expansion: " + reason, CheckTag: "bash.expansion", CheckID: 2001}, nil, nil
	}
	if reason := ExtraSecurityReason(n); reason != "" {
		return tool.PermissionDecision{Behavior: tool.PermissionBehaviorAsk, Reason: "bash security check: " + reason, CheckTag: "bash.security", CheckID: 2002}, nil, nil
	}

	// Hard deny interactive git flows.
	if isInteractiveGit(n) {
		return tool.PermissionDecision{Behavior: tool.PermissionBehaviorDeny, Reason: "interactive git command not supported (-i)", CheckTag: "bash.git.interactive", CheckID: 2101}, nil, nil
	}

	// Ask for explicit confirmation for dangerous operations.
	if isSkipHooks(n) {
		return tool.PermissionDecision{Behavior: tool.PermissionBehaviorAsk, Reason: "skipping hooks requires explicit user request", CheckTag: "bash.git.no_verify", CheckID: 2102}, nil, nil
	}
	if isDestructiveGit(n) {
		return tool.PermissionDecision{Behavior: tool.PermissionBehaviorAsk, Reason: "destructive git command requires explicit user confirmation", CheckTag: "bash.git.destructive", CheckID: 2103}, nil, nil
	}
	if isHighRiskShell(n) {
		return tool.PermissionDecision{Behavior: tool.PermissionBehaviorAsk, Reason: "high-risk shell command requires explicit user confirmation", CheckTag: "bash.shell.high_risk", CheckID: 2104}, nil, nil
	}

	// Informational: destructive warning patterns.
	if w := DestructiveCommandWarning(n); w != "" {
		return tool.PermissionDecision{Behavior: tool.PermissionBehaviorAsk, Reason: "destructive command warning: " + w, CheckTag: "bash.destructive_warning", CheckID: 2105}, nil, nil
	}

	// Otherwise, defer to outer permission rules.
	return tool.PermissionDecision{Behavior: tool.PermissionBehaviorPassthrough}, nil, nil
}

func (t *BashTool) Call(ctx context.Context, input any) (tool.ToolResult, error) {
	fmt.Printf("BashTool.Call: %v\n", input)
	var in Input
	switch v := input.(type) {
	case Input:
		in = v
	case map[string]any:
		b, _ := json.Marshal(v)
		if err := json.Unmarshal(b, &in); err != nil {
			return tool.ToolResult{IsError: true, Content: "invalid input"}, err
		}
	default:
		return tool.ToolResult{IsError: true, Content: "invalid input type"}, fmt.Errorf("invalid input type %T", input)
	}

	timeout := 10 * time.Minute
	if in.TimeoutSeconds > 0 {
		timeout = time.Duration(in.TimeoutSeconds) * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cwd := strings.TrimSpace(in.Cwd)
	if cwd == "" {
		cwd, _ = os.Getwd()
	} else {
		if !filepath.IsAbs(cwd) {
			base, _ := os.Getwd()
			cwd = filepath.Join(base, cwd)
		}
		cwd = filepath.Clean(cwd)
	}

	// Use bash -lc to behave like a login shell for PATH resolution, closer to TS behavior.
	cmd := exec.CommandContext(cctx, "bash", "-lc", in.Command)
	cmd.Dir = cwd

	env := os.Environ()
	env = append(env, "PAGER=cat")
	cmd.Env = env

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Stdin = nil

	err := cmd.Run()

	out := stdout.String()
	errOut := stderr.String()
	combined := strings.TrimRight(out+errOut, "\n")

	// Surface exit status but keep it readable.
	if err != nil {
		return tool.ToolResult{
			IsError: true,
			Content: fmt.Sprintf("Command failed.\nCWD: %s\nError: %s\nOutput:\n%s", cwd, err.Error(), combined),
		}, nil
	}
	if combined == "" {
		combined = "(no output)"
	}
	return tool.ToolResult{
		Content: fmt.Sprintf("CWD: %s\n%s", cwd, combined),
	}, nil
}

func extractCommandString(input any) string {
	switch v := input.(type) {
	case Input:
		return v.Command
	case map[string]any:
		if s, ok := v["command"].(string); ok {
			return s
		}
	}
	return ""
}

func extractCwdString(input any) string {
	switch v := input.(type) {
	case Input:
		return v.Cwd
	case map[string]any:
		if s, ok := v["cwd"].(string); ok {
			return s
		}
	}
	return ""
}

func normalizeCommand(cmd string) string {
	cmd = strings.TrimSpace(cmd)
	// Collapse repeated whitespace for easier substring checks.
	cmd = strings.Join(strings.Fields(cmd), " ")
	return cmd
}

func isSkipHooks(cmd string) bool {
	return strings.Contains(cmd, "--no-verify") || strings.Contains(cmd, "--no-gpg-sign")
}

func isInteractiveGit(cmd string) bool {
	// Match common interactive flags.
	if strings.Contains(cmd, "git rebase -i") || strings.Contains(cmd, "git add -i") {
		return true
	}
	// Any git command with " -i " in arguments.
	if strings.HasPrefix(cmd, "git ") && strings.Contains(cmd, " -i ") {
		return true
	}
	return false
}

func isDestructiveGit(cmd string) bool {
	// Force push variants.
	if strings.Contains(cmd, "git push --force") || strings.Contains(cmd, "git push -f") {
		return true
	}
	// Reset/restore/checkout/clean destructive.
	if strings.Contains(cmd, "git reset --hard") {
		return true
	}
	if strings.Contains(cmd, "git checkout .") || strings.Contains(cmd, "git restore .") {
		return true
	}
	if strings.Contains(cmd, "git clean -f") || strings.Contains(cmd, "git clean -fd") {
		return true
	}
	if strings.Contains(cmd, "git branch -D") {
		return true
	}
	return false
}

func isHighRiskShell(cmd string) bool {
	if strings.Contains(cmd, "rm -rf") || strings.Contains(cmd, "rm -fr") {
		return true
	}
	if strings.HasPrefix(cmd, "sudo ") || strings.Contains(cmd, " sudo ") {
		return true
	}
	if strings.Contains(cmd, "chmod -R") || strings.Contains(cmd, "chown -R") {
		return true
	}
	return false
}
