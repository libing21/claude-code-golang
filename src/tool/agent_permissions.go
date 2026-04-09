package tool

// Go analogs of TS utils/permissions/permissions.ts agent helpers.

// GetDenyRuleForAgent returns a deny rule matching Agent(agentType).
func GetDenyRuleForAgent(ctx PermissionContext, agentToolName string, agentType string) *PermissionRule {
	for i := range ctx.DenyRules {
		r := ctx.DenyRules[i]
		if r.RuleValue.ToolName == agentToolName && r.RuleValue.RuleContent == agentType {
			return &r
		}
	}
	return nil
}

// FilterDeniedAgents filters out agents denied via Agent(agentType) rules.
func FilterDeniedAgents[T interface{ GetAgentType() string }](agents []T, ctx PermissionContext, agentToolName string) []T {
	denied := map[string]struct{}{}
	for _, r := range ctx.DenyRules {
		if r.RuleValue.ToolName == agentToolName && r.RuleValue.RuleContent != "" {
			denied[r.RuleValue.RuleContent] = struct{}{}
		}
	}
	out := make([]T, 0, len(agents))
	for _, a := range agents {
		if _, ok := denied[a.GetAgentType()]; ok {
			continue
		}
		out = append(out, a)
	}
	return out
}

// AllowedAgentTypesFromRules returns a restriction list if Agent(x) allow rules are present.
// Missing => nil (no restriction). Present => only the listed types are allowed.
func AllowedAgentTypesFromRules(ctx PermissionContext, agentToolName string) []string {
	allowed := make([]string, 0, 8)
	for _, r := range ctx.AllowRules {
		if r.RuleValue.ToolName != agentToolName {
			continue
		}
		if r.RuleValue.RuleContent == "" {
			// Tool-wide allow means no type restriction (TS: allowedAgentTypes undefined).
			return nil
		}
		allowed = append(allowed, r.RuleValue.RuleContent)
	}
	if len(allowed) == 0 {
		return nil
	}
	return allowed
}

