package toolruntime

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"claude-code-running-go/src/services/api"
	"claude-code-running-go/src/tool"
	"claude-code-running-go/src/tools"
)

type Options struct {
	MaxConcurrency int
	MaxResultChars int
	Debug          bool
}

func defaultOptions() Options {
	maxC := 4
	if v := strings.TrimSpace(os.Getenv("CLAUDE_CODE_MAX_TOOL_CONCURRENCY")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 64 {
			maxC = n
		}
	}
	return Options{
		MaxConcurrency: maxC,
		MaxResultChars: 20_000,
	}
}

type toolCall struct {
	ID    string
	Name  string
	Input any
}

type batch struct {
	concurrent bool
	calls      []toolCall
}

func canRunConcurrent(t tool.Tool, input any) bool {
	// Mirror TS intent: only run truly safe tool calls concurrently.
	return t.IsConcurrencySafe(input) && t.IsReadOnly(input)
}

func partitionToolCalls(reg *tools.Registry, uses []api.ContentBlock) []batch {
	var out []batch
	var cur *batch

	flush := func() {
		if cur != nil && len(cur.calls) > 0 {
			out = append(out, *cur)
		}
		cur = nil
	}

	for _, u := range uses {
		tc := toolCall{ID: u.ID, Name: u.Name, Input: u.Input}
		timpl, ok := reg.Get(u.Name)
		concurrent := false
		if ok {
			concurrent = canRunConcurrent(timpl, u.Input)
		}

		if cur == nil {
			cur = &batch{concurrent: concurrent}
			cur.calls = append(cur.calls, tc)
			continue
		}
		if cur.concurrent == concurrent {
			cur.calls = append(cur.calls, tc)
			continue
		}
		flush()
		cur = &batch{concurrent: concurrent, calls: []toolCall{tc}}
	}
	flush()
	return out
}

type spillInfo struct {
	Spilled       bool
	Path          string
	OriginalChars int
	ReturnedChars int
}

func truncateOrSpill(id string, content string, maxChars int) (string, spillInfo) {
	info := spillInfo{OriginalChars: len(content), ReturnedChars: len(content)}
	if maxChars <= 0 || len(content) <= maxChars {
		return content, info
	}
	dir := ".claude-go/tool-results"
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, fmt.Sprintf("%s.txt", id))
	_ = os.WriteFile(path, []byte(content), 0o644)
	keep := 4000
	if keep > maxChars {
		keep = maxChars
	}
	out := fmt.Sprintf("%s\n\n(Result truncated. Full content saved to %s)", content[:keep], path)
	return out, spillInfo{
		Spilled:       true,
		Path:          path,
		OriginalChars: len(content),
		ReturnedChars: len(out),
	}
}

func useBlockToolResultContent() bool {
	v, ok := os.LookupEnv("CLAUDE_GO_TOOL_RESULT_BLOCKS")
	if !ok {
		// Default on: needed for tool_reference/defer_loading parity.
		return true
	}
	s := strings.TrimSpace(strings.ToLower(v))
	return s == "1" || s == "true" || s == "yes" || s == "on"
}

func toolResultContent(content any, debugMeta string) any {
	if useBlockToolResultContent() {
		if debugMeta == "" {
			// Pass through blocks if already in block form.
			return content
		}
		switch v := content.(type) {
		case string:
			return []api.TextBlock{
				{Type: "text", Text: debugMeta},
				{Type: "text", Text: v},
			}
		case []any:
			out := make([]any, 0, len(v)+1)
			out = append(out, api.TextBlock{Type: "text", Text: debugMeta})
			out = append(out, v...)
			return out
		default:
			// Fallback: stringify unknown block shapes.
			return []api.TextBlock{
				{Type: "text", Text: debugMeta},
				{Type: "text", Text: fmt.Sprintf("%v", content)},
			}
		}
	}
	// Non-block form: stringify blocks.
	if s, ok := content.(string); ok {
		if debugMeta != "" {
			return debugMeta + "\n" + s
		}
		return s
	}
	if debugMeta != "" {
		return debugMeta + "\n" + fmt.Sprintf("%v", content)
	}
	return fmt.Sprintf("%v", content)
}

func runOne(ctx context.Context, reg *tools.Registry, permCtx tool.PermissionContext, tc toolCall, opts Options) api.ToolResultBlock {
	start := time.Now()
	timpl, ok := reg.Get(tc.Name)
	if !ok {
		meta := ""
		if opts.Debug {
			meta = fmt.Sprintf("[debug] tool=%s reason=unknown_tool", tc.Name)
		}
		return api.ToolResultBlock{
			Type:      "tool_result",
			ToolUseID: tc.ID,
			Content:   toolResultContent(fmt.Sprintf("unknown tool: %s", tc.Name), meta),
			IsError:   true,
		}
	}

	if err := timpl.ValidateInput(tc.Input); err != nil {
		meta := ""
		if opts.Debug {
			meta = fmt.Sprintf("[debug] tool=%s invalid_input=%s", timpl.Name(), err.Error())
		}
		return api.ToolResultBlock{
			Type:      "tool_result",
			ToolUseID: tc.ID,
			Content:   toolResultContent("invalid input: "+err.Error(), meta),
			IsError:   true,
		}
	}

	dec, updated, err := tool.ResolvePermissionDecision(ctx, timpl, tc.Input, permCtx)
	if err != nil {
		meta := ""
		if opts.Debug {
			meta = fmt.Sprintf("[debug] tool=%s perm_err=%s", timpl.Name(), err.Error())
		}
		return api.ToolResultBlock{
			Type:      "tool_result",
			ToolUseID: tc.ID,
			Content:   toolResultContent(err.Error(), meta),
			IsError:   true,
		}
	}
	if dec.Behavior != tool.PermissionBehaviorAllow {
		help := "rerun with --permission-mode bypass, or allow the tool via --allowed-tools " + tc.Name
		meta := ""
		if opts.Debug {
			meta = fmt.Sprintf("[debug] tool=%s perm=%s check_id=%d check_tag=%s reason=%s duration_ms=%d",
				timpl.Name(), dec.Behavior, dec.CheckID, dec.CheckTag, dec.Reason, time.Since(start).Milliseconds())
		}
		return api.ToolResultBlock{
			Type:      "tool_result",
			ToolUseID: tc.ID,
			Content:   toolResultContent(fmt.Sprintf("permission %s: %s (%s)", dec.Behavior, dec.Reason, help), meta),
			IsError:   true,
		}
	}

	callInput := tc.Input
	if updated != nil {
		callInput = updated
	}
	res, callErr := timpl.Call(ctx, callInput)
	if callErr != nil {
		meta := ""
		if opts.Debug {
			meta = fmt.Sprintf("[debug] tool=%s call_err=%s duration_ms=%d", timpl.Name(), callErr.Error(), time.Since(start).Milliseconds())
		}
		return api.ToolResultBlock{
			Type:      "tool_result",
			ToolUseID: tc.ID,
			Content:   toolResultContent(callErr.Error(), meta),
			IsError:   true,
		}
	}
	content := res.Content
	spill := spillInfo{}
	if s, ok := res.Content.(string); ok {
		var trimmed string
		trimmed, spill = truncateOrSpill(tc.ID, s, opts.MaxResultChars)
		content = trimmed
	}
	meta := ""
	if opts.Debug {
		meta = fmt.Sprintf("[debug] tool=%s duration_ms=%d spilled=%v spill_path=%s orig_chars=%d returned_chars=%d",
			timpl.Name(), time.Since(start).Milliseconds(), spill.Spilled, spill.Path, spill.OriginalChars, spill.ReturnedChars)
	}
	return api.ToolResultBlock{
		Type:      "tool_result",
		ToolUseID: tc.ID,
		Content:   toolResultContent(content, meta),
		IsError:   res.IsError,
	}
}

// RunToolCalls executes tool_use blocks into tool_result blocks.
func RunToolCalls(ctx context.Context, reg *tools.Registry, permCtx tool.PermissionContext, uses []api.ContentBlock, o *Options) []api.ToolResultBlock {
	exec := NewStreamingToolExecutor(ctx, reg, permCtx, nil, o)
	return exec.Execute(uses)
}
