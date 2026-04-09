package toolruntime

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ToolUseContext is a minimal Go analog of TS ToolUseContext state.
// It is owned by query loop and passed into tool execution.
type ToolUseContext struct {
	abortCtx    context.Context
	abortCancel context.CancelCauseFunc

	mu                        sync.Mutex
	inProgressToolUseIDs      map[string]struct{}
	hasInterruptibleInProgress bool
	progressEvents            []ProgressEvent
}

type interruptCause struct{}

func (interruptCause) Error() string { return "interrupt" }

type ProgressStage string

const (
	ProgressQueued    ProgressStage = "queued"
	ProgressExecuting ProgressStage = "executing"
	ProgressCompleted ProgressStage = "completed"
)

type ProgressEvent struct {
	TimeMs    int64         `json:"timeMs"`
	ToolUseID string        `json:"toolUseId"`
	ToolName  string        `json:"toolName"`
	Stage     ProgressStage `json:"stage"`
	Message   string        `json:"message"`
}

func NewToolUseContext(ctx context.Context) *ToolUseContext {
	abortCtx, abortCancel := context.WithCancelCause(ctx)
	return &ToolUseContext{
		abortCtx:            abortCtx,
		abortCancel:         abortCancel,
		inProgressToolUseIDs: map[string]struct{}{},
		progressEvents:       []ProgressEvent{},
	}
}

func (c *ToolUseContext) AbortContext() context.Context { return c.abortCtx }

func (c *ToolUseContext) AbortReason() string {
	cause := context.Cause(c.abortCtx)
	if cause == nil {
		return ""
	}
	var ic interruptCause
	if errors.As(cause, &ic) {
		return "interrupt"
	}
	return "cancelled"
}

func (c *ToolUseContext) Interrupt() {
	// Do not cancel the parent turn; only cancel this tool-use context.
	c.abortCancel(interruptCause{})
}

func (c *ToolUseContext) MarkQueued(toolUseID string, toolName string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.progressEvents = append(c.progressEvents, ProgressEvent{
		TimeMs:    time.Now().UnixMilli(),
		ToolUseID: toolUseID,
		ToolName:  toolName,
		Stage:     ProgressQueued,
		Message:   "queued",
	})
}

func (c *ToolUseContext) MarkExecuting(toolUseID string, toolName string, msg string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.inProgressToolUseIDs[toolUseID] = struct{}{}
	c.progressEvents = append(c.progressEvents, ProgressEvent{
		TimeMs:    time.Now().UnixMilli(),
		ToolUseID: toolUseID,
		ToolName:  toolName,
		Stage:     ProgressExecuting,
		Message:   msg,
	})
}

func (c *ToolUseContext) MarkCompleted(toolUseID string, toolName string, msg string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.inProgressToolUseIDs, toolUseID)
	c.progressEvents = append(c.progressEvents, ProgressEvent{
		TimeMs:    time.Now().UnixMilli(),
		ToolUseID: toolUseID,
		ToolName:  toolName,
		Stage:     ProgressCompleted,
		Message:   msg,
	})
}

func (c *ToolUseContext) SetHasInterruptibleToolInProgress(v bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.hasInterruptibleInProgress = v
}

func (c *ToolUseContext) HasInterruptibleToolInProgress() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.hasInterruptibleInProgress
}

func (c *ToolUseContext) ProgressEvents() []ProgressEvent {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]ProgressEvent, len(c.progressEvents))
	copy(out, c.progressEvents)
	return out
}
