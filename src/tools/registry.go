package tools

import (
	"claude-code-running-go/src/tool"
)

// Registry holds the available tool implementations.
type Registry struct {
	toolsByName map[string]tool.Tool
	ordered     []tool.Tool
}

func NewRegistry(ts []tool.Tool) *Registry {
	m := make(map[string]tool.Tool, len(ts))
	ordered := make([]tool.Tool, 0, len(ts))
	for _, t := range ts {
		m[t.Name()] = t
		ordered = append(ordered, t)
	}
	return &Registry{toolsByName: m, ordered: ordered}
}

func (r *Registry) Get(name string) (tool.Tool, bool) {
	t, ok := r.toolsByName[name]
	return t, ok
}

func (r *Registry) Add(t tool.Tool) {
	// Overwrite-by-name semantics; keep insertion order append for stable exposure.
	r.toolsByName[t.Name()] = t
	r.ordered = append(r.ordered, t)
}

func (r *Registry) List() []tool.Tool {
	out := make([]tool.Tool, 0, len(r.ordered))
	out = append(out, r.ordered...)
	return out
}
