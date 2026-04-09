package toolruntime

import (
	"encoding/json"
	"sort"
	"strings"

	"claude-code-running-go/src/services/api"
	"claude-code-running-go/src/tool"
)

// ToolOrchestration is a minimal Go analog of TS services/tools/toolOrchestration.
// It supports defer-loading: only "always" tools are exposed initially; deferred tools
// (e.g. MCP tools) are exposed only after they are discovered via tool_reference blocks.
type ToolOrchestration struct {
	ExposedTools []tool.Tool

	// If false, all exposed tools are sent every request (legacy behavior).
	DeferLoading bool

	// toolName -> tool implementation (only for deferred tools)
	deferred map[string]tool.Tool
	// toolName set
	discovered map[string]struct{}

	// Persist/replay discovered state across restarts/compaction.
	store       DiscoveredToolsStore
	maxKeep     int
	discoverOrd []string // insertion order; used for budget-based eviction
	dirty       bool
}

func NewToolOrchestration(exposed []tool.Tool) *ToolOrchestration {
	o := &ToolOrchestration{
		ExposedTools: exposed,
		deferred:     map[string]tool.Tool{},
		discovered:   map[string]struct{}{},
		maxKeep:      256,
	}
	for _, t := range exposed {
		if isDeferredTool(t) {
			o.deferred[t.Name()] = t
		}
	}
	return o
}

func isDeferredTool(t tool.Tool) bool {
	// TS: isDeferredTool = MCP tools + shouldDefer tools. Go port supports both:
	// - IsMCPTool() marker for MCP tools
	// - IsDeferredTool() opt-in marker for other tools
	if mt, ok := t.(interface{ IsMCPTool() bool }); ok && mt.IsMCPTool() {
		return true
	}
	if dt, ok := t.(interface{ IsDeferredTool() bool }); ok && dt.IsDeferredTool() {
		return true
	}
	return false
}

func (o *ToolOrchestration) BuildToolSchemas() []api.ToolSchema {
	tools := make([]tool.Tool, 0, len(o.ExposedTools))
	if !o.DeferLoading {
		tools = append(tools, o.ExposedTools...)
	} else {
		for _, t := range o.ExposedTools {
			if _, isDeferred := o.deferred[t.Name()]; isDeferred {
				if _, ok := o.discovered[t.Name()]; !ok {
					continue
				}
			}
			tools = append(tools, t)
		}
	}

	schemas := make([]api.ToolSchema, 0, len(tools))
	for _, t := range tools {
		schemas = append(schemas, api.ToolSchema{
			Name:        t.Name(),
			Description: t.Prompt(),
			InputSchema: t.InputSchema(),
		})
	}
	// Stable ordering helps avoid prompt churn (TS behavior).
	sort.Slice(schemas, func(i, j int) bool { return schemas[i].Name < schemas[j].Name })
	return schemas
}

func (o *ToolOrchestration) UpdateDiscoveredFromToolResults(results []api.ToolResultBlock) {
	if !o.DeferLoading {
		return
	}
	for _, r := range results {
		for _, name := range ExtractToolReferenceNames(r.Content) {
			// Only accept tool references that exist in deferred pool.
			if _, ok := o.deferred[name]; ok {
				o.addDiscovered(name)
			}
		}
	}
	o.cleanupDiscovered()
}

func (o *ToolOrchestration) DiscoveredDeferredNames() []string {
	out := make([]string, 0, len(o.discovered))
	for n := range o.discovered {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func (o *ToolOrchestration) SetDiscoveredToolsStore(store DiscoveredToolsStore, maxKeep int) {
	o.store = store
	if maxKeep > 0 {
		o.maxKeep = maxKeep
	}
}

// LoadDiscovered seeds the discovered set from a store if configured.
// Only names that still exist in the deferred pool are kept.
func (o *ToolOrchestration) LoadDiscovered() error {
	if o.store == nil {
		return nil
	}
	names, err := o.store.Load()
	if err != nil {
		return err
	}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, ok := o.deferred[n]; !ok {
			continue
		}
		o.addDiscovered(n)
	}
	o.dirty = false
	return nil
}

// SaveDiscovered persists the discovered set if it changed.
func (o *ToolOrchestration) SaveDiscovered() error {
	if o.store == nil || !o.dirty {
		return nil
	}
	names := o.DiscoveredDeferredNames()
	if err := o.store.Save(names); err != nil {
		return err
	}
	o.dirty = false
	return nil
}

// ReplayDiscoveredFromMessages rebuilds discovered state by scanning transcript messages.
// This is used when store is unavailable or after compaction boundaries.
func (o *ToolOrchestration) ReplayDiscoveredFromMessages(msgs []api.Message) {
	if !o.DeferLoading {
		return
	}
	for _, msg := range msgs {
		if msg.Role != "user" {
			continue
		}
		blocks, ok := msg.Content.([]map[string]any)
		if !ok {
			continue
		}
		for _, b := range blocks {
			typ, _ := b["type"].(string)
			if typ != "tool_result" {
				continue
			}
			for _, name := range ExtractToolReferenceNames(b["content"]) {
				if _, ok := o.deferred[name]; ok {
					o.addDiscovered(name)
				}
			}
		}
	}
	o.cleanupDiscovered()
}

func (o *ToolOrchestration) addDiscovered(name string) {
	if _, ok := o.discovered[name]; ok {
		return
	}
	o.discovered[name] = struct{}{}
	o.discoverOrd = append(o.discoverOrd, name)
	o.dirty = true
	if len(o.discoverOrd) > o.maxKeep {
		// Evict the oldest entries (LRU-ish by insertion order).
		excess := len(o.discoverOrd) - o.maxKeep
		for i := 0; i < excess; i++ {
			old := o.discoverOrd[i]
			delete(o.discovered, old)
		}
		o.discoverOrd = append([]string{}, o.discoverOrd[excess:]...)
	}
}

func (o *ToolOrchestration) cleanupDiscovered() {
	// Drop discovered names that no longer exist in deferred pool.
	if len(o.discovered) == 0 {
		return
	}
	cleanOrd := make([]string, 0, len(o.discoverOrd))
	for _, n := range o.discoverOrd {
		if _, ok := o.deferred[n]; !ok {
			delete(o.discovered, n)
			o.dirty = true
			continue
		}
		if _, ok := o.discovered[n]; ok {
			cleanOrd = append(cleanOrd, n)
		}
	}
	o.discoverOrd = cleanOrd
}

// ExtractToolReferenceNames scans tool_result content for blocks like:
// { "type": "tool_reference", "tool_name": "<name>" }
func ExtractToolReferenceNames(content any) []string {
	out := make([]string, 0)
	arr, ok := content.([]any)
	if !ok {
		// Could be []map[string]any after json round-trip; try that too.
		if ms, ok := content.([]map[string]any); ok {
			for _, m := range ms {
				typ, _ := m["type"].(string)
				if typ != "tool_reference" {
					continue
				}
				name, _ := m["tool_name"].(string)
				if name != "" {
					out = append(out, name)
				}
			}
			return out
		}
		// Best-effort: some callers store as []interface{} but behind a json.RawMessage.
		if raw, ok := content.(json.RawMessage); ok && len(raw) > 0 {
			var tmp []map[string]any
			if err := json.Unmarshal(raw, &tmp); err == nil {
				for _, m := range tmp {
					typ, _ := m["type"].(string)
					if typ != "tool_reference" {
						continue
					}
					name, _ := m["tool_name"].(string)
					if name != "" {
						out = append(out, name)
					}
				}
			}
		}
		return out
	}
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			// api.TextBlock may appear when debug meta is prepended.
			if tb, ok := it.(api.TextBlock); ok {
				_ = tb
			}
			continue
		}
		typ, _ := m["type"].(string)
		if typ != "tool_reference" {
			continue
		}
		name, _ := m["tool_name"].(string)
		if name != "" {
			out = append(out, name)
		}
	}
	return out
}
