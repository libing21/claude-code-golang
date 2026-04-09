package attachments

import (
	"sort"

	"claude-code-running-go/src/services/api"
)

// This package is a Go-structured analog of TS src/utils/attachments.ts.
// The Anthropic SDK "attachments" field is not implemented in this Go port yet,
// so we represent attachments as synthetic messages (Role=user) injected into
// the message list. This preserves behavior and keeps it debuggable.

type Kind string

type Context struct {
	// Add fields as we port more attachment types.
	McpInstructionsSection string
	McpInstructionsDelta   bool
	RelevantMemories       []RelevantMemoryAttachment
	CriticalSystemReminder string
	OutputStyleName        string
	DateChanged            bool
	NewDate                string
	PlanMode               *PlanModeAttachment
	PlanModeReentry        *PlanModeReentryAttachment
	PlanModeExit           *PlanModeExitAttachment
	AutoMode               *AutoModeAttachment
	AutoModeExit           bool
	DeferredToolsDelta     *DeferredToolsDeltaAttachment
	AgentListingDelta      *AgentListingDeltaAttachment
}

type Provider interface {
	Kind() Kind
	// Priority controls ordering; lower runs earlier (TS keeps stable ordering).
	Priority() int
	Build(ctx Context) []api.Message
}

type Registry struct {
	providers []Provider
}

func NewRegistry() *Registry { return &Registry{} }

func (r *Registry) Register(p Provider) {
	r.providers = append(r.providers, p)
	// Keep stable order by priority then kind to reduce diff churn.
	sort.SliceStable(r.providers, func(i, j int) bool {
		pi, pj := r.providers[i], r.providers[j]
		if pi.Priority() != pj.Priority() {
			return pi.Priority() < pj.Priority()
		}
		return string(pi.Kind()) < string(pj.Kind())
	})
}

func (r *Registry) BuildMessages(ctx Context) []api.Message {
	out := make([]api.Message, 0)
	for _, p := range r.providers {
		out = append(out, p.Build(ctx)...)
	}
	return out
}
