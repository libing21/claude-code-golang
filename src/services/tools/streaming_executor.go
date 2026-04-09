package toolruntime

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"sort"
	"strings"
	"sync"

	"claude-code-running-go/src/services/api"
	"claude-code-running-go/src/tool"
	"claude-code-running-go/src/tools"
)

// StreamingToolExecutor is a minimal Go analog of TS StreamingToolExecutor.
// Our Go query loop is not streaming tool_use blocks incrementally yet; this executor
// still provides the same semantics for a batch of tool_use blocks:
// - Concurrency-safe tools can run in parallel up to MaxConcurrency
// - Non-concurrent tools execute exclusively (wait for all in-flight, then run serially)
// - Results are returned in tool-use order
// - If a Bash tool errors, sibling concurrent executions are cancelled (best-effort)
type StreamingToolExecutor struct {
	reg     *tools.Registry
	permCtx tool.PermissionContext
	opts    Options
	tuc     *ToolUseContext

	// siblingCtx is cancelled when a sibling error should stop other concurrent tools.
	siblingCtx    context.Context
	siblingCancel context.CancelFunc

	// Remember which tool triggered sibling abort for better messaging.
	siblingErroredToolDesc string
	siblingErroredToolUseID string
	siblingAborted         bool
	mu                     sync.Mutex
	executingTotal         int
	executingCancelable    int
}

func NewStreamingToolExecutor(ctx context.Context, reg *tools.Registry, permCtx tool.PermissionContext, tuc *ToolUseContext, o *Options) *StreamingToolExecutor {
	opts := defaultOptions()
	if o != nil {
		if o.MaxConcurrency > 0 {
			opts.MaxConcurrency = o.MaxConcurrency
		}
		if o.MaxResultChars > 0 {
			opts.MaxResultChars = o.MaxResultChars
		}
		if o.Debug {
			opts.Debug = true
		}
	}
	sibCtx, cancel := context.WithCancel(ctx)
	if tuc == nil {
		tuc = NewToolUseContext(ctx)
	}
	return &StreamingToolExecutor{
		reg:           reg,
		permCtx:       permCtx,
		opts:          opts,
		tuc:          tuc,
		siblingCtx:    sibCtx,
		siblingCancel: cancel,
	}
}

func (e *StreamingToolExecutor) updateInterruptibleStateLocked() {
	// Mirror TS: "interruptible" only when all executing tools are cancelable.
	e.tuc.SetHasInterruptibleToolInProgress(e.executingTotal > 0 && e.executingTotal == e.executingCancelable)
}

func (e *StreamingToolExecutor) toolInterruptBehavior(name string, input any) tool.InterruptBehavior {
	timpl, ok := e.reg.Get(name)
	if !ok {
		return tool.InterruptBehaviorBlock
	}
	if ib, ok := timpl.(tool.InterruptBehaviorProvider); ok {
		defer func() { _ = recover() }()
		b := ib.InterruptBehavior(input)
		if b == tool.InterruptBehaviorCancel {
			return b
		}
		return tool.InterruptBehaviorBlock
	}
	return tool.InterruptBehaviorBlock
}

func (e *StreamingToolExecutor) toolDescription(name string, input any) string {
	// TS shows a short summary. We only include some common fields.
	m, ok := input.(map[string]any)
	if ok {
		for _, k := range []string{"command", "file_path", "pattern"} {
			if v, ok := m[k].(string); ok && strings.TrimSpace(v) != "" {
				s := strings.TrimSpace(v)
				if len(s) > 40 {
					s = s[:40] + "..."
				}
				return fmt.Sprintf("%s(%s)", name, s)
			}
		}
	}
	return name
}

func (e *StreamingToolExecutor) recordSiblingAbort(toolUseID string, toolDesc string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.siblingAborted {
		return
	}
	e.siblingAborted = true
	e.siblingErroredToolDesc = toolDesc
	e.siblingErroredToolUseID = toolUseID
	e.siblingCancel()
}

func (e *StreamingToolExecutor) syntheticCancelResult(toolUseID string, reason string) api.ToolResultBlock {
	// Keep close to TS wording; this is a non-UI runtime port.
	msg := ""
	switch reason {
	case "sibling_error":
		if e.siblingErroredToolDesc != "" {
			msg = "Cancelled: parallel tool call " + e.siblingErroredToolDesc + " errored"
		} else {
			msg = "Cancelled: parallel tool call errored"
		}
	case "user_interrupted":
		msg = "Cancelled: user interrupted tool execution"
	default:
		msg = "Cancelled"
	}
	return api.ToolResultBlock{
		Type:      "tool_result",
		ToolUseID: toolUseID,
		Content:   toolResultContent("<tool_use_error>"+msg+"</tool_use_error>", ""),
		IsError:   true,
	}
}

func (e *StreamingToolExecutor) isConcurrencySafe(tc toolCall) bool {
	timpl, ok := e.reg.Get(tc.Name)
	if !ok {
		return true // unknown tool: doesn't mutate state; treat as safe
	}
	defer func() {
		if r := recover(); r != nil {
			// Conservative: if tool's predicates panic, do not run concurrently.
			_ = r
		}
	}()
	return canRunConcurrent(timpl, tc.Input)
}

func (e *StreamingToolExecutor) runTool(tc toolCall) api.ToolResultBlock {
	// Mark executing for progress + interruptible state.
	ib := e.toolInterruptBehavior(tc.Name, tc.Input)
	e.mu.Lock()
	e.executingTotal++
	if ib == tool.InterruptBehaviorCancel {
		e.executingCancelable++
	}
	e.updateInterruptibleStateLocked()
	e.mu.Unlock()

	e.tuc.MarkExecuting(tc.ID, tc.Name, "executing")
	defer func() {
		e.mu.Lock()
		e.executingTotal--
		if ib == tool.InterruptBehaviorCancel {
			e.executingCancelable--
		}
		e.updateInterruptibleStateLocked()
		e.mu.Unlock()
	}()

	// Use siblingCtx for best-effort cancellation between concurrent tools.
	// Additionally, respect user interrupt only for cancelable tools.
	ctxUsed := e.siblingCtx
		if ib == tool.InterruptBehaviorCancel {
			abortCtx := e.tuc.AbortContext()
			// Merge abortCtx with siblingCtx so this tool can be cancelled.
			merged, cancel := context.WithCancel(e.siblingCtx)
			done := merged.Done()
			go func() {
				select {
				case <-abortCtx.Done():
					cancel()
				case <-done:
				}
			}()
			defer cancel()
			ctxUsed = merged
		}
	if e.opts.Debug {
		fmt.Fprintf(os.Stderr, "[tool] start id=%s name=%s ib=%s\n", tc.ID, tc.Name, ib)
	}
	res := runOne(ctxUsed, e.reg, e.permCtx, tc, e.opts)
	if e.opts.Debug {
		fmt.Fprintf(os.Stderr, "[tool] end   id=%s name=%s is_error=%v sib_ctx_err=%v\n", tc.ID, tc.Name, res.IsError, e.siblingCtx.Err())
	}
	if res.IsError && tc.Name == "Bash" {
		// TS sibling-cancel behavior: a failing Bash aborts other concurrent tools.
		// IMPORTANT: do not mask the failing Bash tool's own error as "Cancelled".
		e.recordSiblingAbort(tc.ID, e.toolDescription(tc.Name, tc.Input))
	}
	// If this tool was cancelled because a sibling aborted, normalize to synthetic message,
	// but only for the sibling tools, not the tool that actually errored.
	if e.siblingCtx.Err() != nil {
		e.mu.Lock()
		sib := e.siblingAborted
		erroredID := e.siblingErroredToolUseID
		e.mu.Unlock()
		if sib && res.IsError && tc.ID != erroredID && ctxUsed.Err() != nil {
			// runOne likely emitted context-canceled; prefer stable messaging.
			return e.syntheticCancelResult(tc.ID, "sibling_error")
		}
	}
	if res.IsError {
		e.tuc.MarkCompleted(tc.ID, tc.Name, "completed (error)")
	} else {
		e.tuc.MarkCompleted(tc.ID, tc.Name, "completed")
	}
	return res
}

func (e *StreamingToolExecutor) Execute(uses []api.ContentBlock) []api.ToolResultBlock {
	// Flatten tool_use blocks into toolCall list.
	calls := make([]toolCall, 0, len(uses))
	for _, u := range uses {
		calls = append(calls, toolCall{ID: u.ID, Name: u.Name, Input: u.Input})
		e.tuc.MarkQueued(u.ID, u.Name)
	}

	// Execution state.
	results := make([]api.ToolResultBlock, len(calls))
	sem := make(chan struct{}, e.opts.MaxConcurrency)

	type inFlight struct {
		idx int
		tc  toolCall
	}
	inFlights := make([]inFlight, 0, e.opts.MaxConcurrency)
	var wg sync.WaitGroup

	flush := func() {
		wg.Wait()
		inFlights = inFlights[:0]
	}

	startConcurrent := func(idx int, tc toolCall) {
		wg.Add(1)
		inFlights = append(inFlights, inFlight{idx: idx, tc: tc})
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			defer func() {
				if r := recover(); r != nil {
					// Convert panics into a tool_result error (keeps executor robust).
					msg := fmt.Sprintf("tool panicked: %v", r)
					if e.opts.Debug {
						msg += "\n" + string(debug.Stack())
					}
					results[idx] = api.ToolResultBlock{
						Type:      "tool_result",
						ToolUseID: tc.ID,
						Content:   toolResultContent(msg, ""),
						IsError:   true,
					}
				}
			}()
			results[idx] = e.runTool(tc)
		}()
	}

	for i, tc := range calls {
		// If tool-use context was interrupted, only cancel cancelable tools.
		if e.tuc.AbortContext().Err() != nil && e.tuc.AbortReason() == "interrupt" {
			if e.toolInterruptBehavior(tc.Name, tc.Input) == tool.InterruptBehaviorCancel {
				e.tuc.MarkCompleted(tc.ID, tc.Name, "cancelled (interrupt)")
				results[i] = e.syntheticCancelResult(tc.ID, "user_interrupted")
				continue
			}
			// block tools continue despite interrupt
		}

		// If parent ctx is canceled, synthesize errors for the rest.
		if e.siblingCtx.Err() != nil {
			e.mu.Lock()
			sib := e.siblingAborted
			e.mu.Unlock()
			reason := "user_interrupted"
			if sib {
				reason = "sibling_error"
			}
			results[i] = e.syntheticCancelResult(tc.ID, reason)
			continue
		}

		concurrent := e.isConcurrencySafe(tc)
		if !concurrent {
			// Exclusive tool: wait for all concurrent tools to finish first.
			flush()
			results[i] = e.runTool(tc)
			continue
		}
		startConcurrent(i, tc)
	}

	flush()

	// Ensure stable order and no zero-values (shouldn't happen, but be safe).
	for i := range results {
		if results[i].Type == "" {
			results[i] = api.ToolResultBlock{
				Type:      "tool_result",
				ToolUseID: calls[i].ID,
				Content:   toolResultContent("internal error: missing tool result", ""),
				IsError:   true,
			}
		}
	}

	// Some call sites depend on stable tool_result order matching tool_use order.
	sort.SliceStable(results, func(i, j int) bool { return i < j })
	return results
}
