package tool

import (
	"context"
	"fmt"
	"strings"
)

func normalizeRuleToolName(name string) string {
	return strings.TrimSpace(name)
}

// PermissionRuleValueFromString mirrors the TS parser shape:
// "Bash" or "Bash(git status)".
func PermissionRuleValueFromString(ruleString string) PermissionRuleValue {
	ruleString = strings.TrimSpace(ruleString)
	open := strings.Index(ruleString, "(")
	close := strings.LastIndex(ruleString, ")")
	if open <= 0 || close != len(ruleString)-1 || close <= open {
		return PermissionRuleValue{ToolName: normalizeRuleToolName(ruleString)}
	}
	toolName := normalizeRuleToolName(ruleString[:open])
	rawContent := ruleString[open+1 : close]
	if rawContent == "" || rawContent == "*" {
		return PermissionRuleValue{ToolName: toolName}
	}
	return PermissionRuleValue{ToolName: toolName, RuleContent: rawContent}
}

func BuildRulesFromStrings(source string, behavior PermissionBehavior, raw []string) []PermissionRule {
	out := make([]PermissionRule, 0, len(raw))
	for _, s := range raw {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		out = append(out, PermissionRule{
			Source:       source,
			RuleBehavior: behavior,
			RuleValue:    PermissionRuleValueFromString(s),
		})
	}
	return out
}

func toolMatchesRule(tool Tool, rule PermissionRule) bool {
	// Tool-wide rules only for now. Content rules are reserved for tool-specific checks.
	if rule.RuleValue.RuleContent != "" {
		return false
	}
	return rule.RuleValue.ToolName == tool.Name()
}

func toolAlwaysAllowedRule(permCtx PermissionContext, t Tool) *PermissionRule {
	for i := range permCtx.AllowRules {
		if toolMatchesRule(t, permCtx.AllowRules[i]) {
			return &permCtx.AllowRules[i]
		}
	}
	return nil
}

func getDenyRuleForTool(permCtx PermissionContext, t Tool) *PermissionRule {
	for i := range permCtx.DenyRules {
		if toolMatchesRule(t, permCtx.DenyRules[i]) {
			return &permCtx.DenyRules[i]
		}
	}
	return nil
}

func getAskRuleForTool(permCtx PermissionContext, t Tool) *PermissionRule {
	for i := range permCtx.AskRules {
		if toolMatchesRule(t, permCtx.AskRules[i]) {
			return &permCtx.AskRules[i]
		}
	}
	return nil
}

// ResolvePermissionDecision applies the same broad ordering as the TS runtime:
// deny -> ask -> tool-specific -> bypass -> allow -> default ask.
func ResolvePermissionDecision(
	ctx context.Context,
	t Tool,
	input any,
	permCtx PermissionContext,
) (PermissionDecision, any, error) {
	select {
	case <-ctx.Done():
		return PermissionDecision{
			Behavior: PermissionBehaviorDeny,
			Reason:   "cancelled",
		}, nil, ctx.Err()
	default:
	}

	if denyRule := getDenyRuleForTool(permCtx, t); denyRule != nil {
		return PermissionDecision{
			Behavior: PermissionBehaviorDeny,
			Reason:   fmt.Sprintf("denied by %s rule", denyRule.Source),
			CheckTag: "rule.deny",
			CheckID:  1001,
		}, nil, nil
	}

	if askRule := getAskRuleForTool(permCtx, t); askRule != nil {
		return PermissionDecision{
			Behavior: PermissionBehaviorAsk,
			Reason:   fmt.Sprintf("ask by %s rule", askRule.Source),
			CheckTag: "rule.ask",
			CheckID:  1002,
		}, nil, nil
	}

	toolDecision, updatedInput, err := t.CheckPermissions(ctx, input, permCtx)
	if err != nil {
		return PermissionDecision{
			Behavior: PermissionBehaviorDeny,
			Reason:   err.Error(),
		}, updatedInput, err
	}

	if toolDecision.Behavior == PermissionBehaviorDeny {
		return toolDecision, updatedInput, nil
	}
	if toolDecision.Behavior == PermissionBehaviorAsk {
		return toolDecision, updatedInput, nil
	}
	// Passthrough means the tool itself has no strong opinion; higher-level policy decides.
	if toolDecision.Behavior == PermissionBehaviorAllow {
		// Tool-level allow is an explicit allow (used for low-risk read-only tools).
		return toolDecision, updatedInput, nil
	}

	if permCtx.Mode == PermissionModeBypass {
		return PermissionDecision{
			Behavior: PermissionBehaviorAllow,
			Reason:   "allowed by mode=bypass",
			CheckTag: "mode.bypass",
			CheckID:  1101,
		}, updatedInput, nil
	}

	if allowRule := toolAlwaysAllowedRule(permCtx, t); allowRule != nil {
		return PermissionDecision{
			Behavior: PermissionBehaviorAllow,
			Reason:   fmt.Sprintf("allowed by %s rule", allowRule.Source),
			CheckTag: "rule.allow",
			CheckID:  1000,
		}, updatedInput, nil
	}

	if permCtx.Mode == PermissionModeAsk {
		return PermissionDecision{
			Behavior: PermissionBehaviorAsk,
			Reason:   "mode=ask",
			CheckTag: "mode.ask",
			CheckID:  1102,
		}, updatedInput, nil
	}

	// Default mirrors TS passthrough -> ask behavior.
	return PermissionDecision{
		Behavior: PermissionBehaviorAsk,
		Reason:   "default prompt required",
		CheckTag: "default.ask",
		CheckID:  1999,
	}, updatedInput, nil
}
