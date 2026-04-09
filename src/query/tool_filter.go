package query

import (
	"claude-code-running-go/src/tool"
)

type toolExposure struct {
	allowAll   bool
	allowedSet map[string]struct{}
	denySet    map[string]struct{}
}

func buildToolExposure(allowedRaw []string, disallowedRaw []string) toolExposure {
	te := toolExposure{
		allowAll:   allowedRaw == nil,
		allowedSet: map[string]struct{}{},
		denySet:    map[string]struct{}{},
	}
	// allowedRaw:
	// - nil  => no restriction (allow all)
	// - []   => explicitly allow none (agent frontmatter empty list)
	// - list => allow only listed tool names (rule content is ignored for exposure)
	if allowedRaw != nil {
		for _, s := range allowedRaw {
			rv := tool.PermissionRuleValueFromString(s)
			if rv.ToolName == "*" {
				te.allowAll = true
				continue
			}
			if rv.ToolName != "" {
				te.allowedSet[rv.ToolName] = struct{}{}
			}
		}
	}
	for _, s := range disallowedRaw {
		rv := tool.PermissionRuleValueFromString(s)
		if rv.ToolName != "" {
			te.denySet[rv.ToolName] = struct{}{}
		}
	}
	return te
}

func (te toolExposure) IsExposed(toolName string) bool {
	if _, ok := te.denySet[toolName]; ok {
		return false
	}
	if te.allowAll {
		return true
	}
	// allowed list present but empty => expose nothing
	if len(te.allowedSet) == 0 {
		return false
	}
	_, ok := te.allowedSet[toolName]
	return ok
}

func filterToolsForSchema(tools []tool.Tool, allowedRaw []string, disallowedRaw []string) []tool.Tool {
	te := buildToolExposure(allowedRaw, disallowedRaw)
	out := make([]tool.Tool, 0, len(tools))
	for _, t := range tools {
		if te.IsExposed(t.Name()) {
			out = append(out, t)
		}
	}
	return out
}

