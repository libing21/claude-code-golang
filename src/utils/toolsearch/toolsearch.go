package toolsearch

import (
	"math"
	"os"
	"strconv"
	"strings"

	"claude-code-running-go/src/tool"
	"claude-code-running-go/src/utils/model"
)

// Go port of TS utils/toolSearch.ts gating for non-UI runtime parity.
// This is still intentionally smaller than TS (no GrowthBook/token-counter API),
// but it matches the behavioral envelope:
// - ENABLE_TOOL_SEARCH mode parsing (tst / tst-auto / standard)
// - optimistic gate for first-party provider + non-first-party base URL
// - modelSupportsToolReference negative list (defaults: haiku)
// - definitive gate includes ToolSearch tool availability + auto threshold

type Mode string

const (
	ModeTST     Mode = "tst"
	ModeTSTAuto Mode = "tst-auto"
	ModeStandard Mode = "standard"
)

func envTruthy(key string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func envDefinedFalsy(key string) bool {
	v, ok := os.LookupEnv(key)
	if !ok {
		return false
	}
	v = strings.TrimSpace(strings.ToLower(v))
	return v == "" || v == "0" || v == "false" || v == "off" || v == "no"
}

const (
	defaultAutoToolSearchPercentage = 10  // 10%
	charsPerToken                   = 2.5 // TS heuristic
)

func parseAutoPercentage(value string) (int, bool) {
	if !strings.HasPrefix(value, "auto:") {
		return 0, false
	}
	raw := strings.TrimSpace(strings.TrimPrefix(value, "auto:"))
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	if n < 0 {
		n = 0
	}
	if n > 100 {
		n = 100
	}
	return n, true
}

func isAutoToolSearchMode(value string) bool {
	return value == "auto" || strings.HasPrefix(value, "auto:")
}

func getAutoToolSearchPercentage() int {
	value := strings.TrimSpace(os.Getenv("ENABLE_TOOL_SEARCH"))
	if value == "" {
		return defaultAutoToolSearchPercentage
	}
	if value == "auto" {
		return defaultAutoToolSearchPercentage
	}
	if n, ok := parseAutoPercentage(value); ok {
		return n
	}
	return defaultAutoToolSearchPercentage
}

func getContextWindowTokensForModel(_ string) int {
	// TS: getContextWindowForModel(model, betas). Go port uses a conservative default.
	// Allow override for testing/proxies.
	if v := strings.TrimSpace(os.Getenv("CLAUDE_GO_TOOL_SEARCH_CONTEXT_WINDOW_TOKENS")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	// Most Claude models are 200k-class in the CLI context; the exact value
	// matters only for auto threshold, not correctness.
	return 200_000
}

func getAutoToolSearchCharThreshold(model string) int {
	percentage := float64(getAutoToolSearchPercentage()) / 100.0
	tokens := float64(getContextWindowTokensForModel(model)) * percentage
	return int(math.Floor(tokens * charsPerToken))
}

func GetMode() Mode {
	// TS: kill switch for beta shapes.
	if envTruthy("CLAUDE_CODE_DISABLE_EXPERIMENTAL_BETAS") {
		return ModeStandard
	}
	v := strings.TrimSpace(strings.ToLower(os.Getenv("ENABLE_TOOL_SEARCH")))
	if v == "" {
		return ModeTST // default: on
	}
	if isAutoToolSearchMode(v) {
		// auto:0 = always enabled, auto:100 = standard.
		if n, ok := parseAutoPercentage(v); ok && n == 0 {
			return ModeTST
		}
		if n, ok := parseAutoPercentage(v); ok && n == 100 {
			return ModeStandard
		}
		// auto or auto:1-99
		return ModeTSTAuto
	}
	if v == "true" || v == "1" || v == "yes" || v == "on" {
		return ModeTST
	}
	if envDefinedFalsy("ENABLE_TOOL_SEARCH") {
		return ModeStandard
	}
	return ModeTST
}

func ModelSupportsToolReference(model string) bool {
	// TS uses a negative list (via GrowthBook). Go port supports an env override.
	// New models are assumed to support tool_reference unless explicitly listed.
	m := strings.ToLower(strings.TrimSpace(model))

	// Allow comma-separated override list: e.g. "haiku,foo".
	if v := strings.TrimSpace(os.Getenv("CLAUDE_GO_TOOL_SEARCH_UNSUPPORTED_MODELS")); v != "" {
		for _, p := range strings.Split(v, ",") {
			p = strings.ToLower(strings.TrimSpace(p))
			if p == "" {
				continue
			}
			if strings.Contains(m, p) {
				return false
			}
		}
		return true
	}

	return !strings.Contains(m, "haiku")
}

func IsEnabledOptimistic() bool {
	mode := GetMode()
	if mode == ModeStandard {
		return false
	}

	// TS: optimistic proxy gate. Only applies when ENABLE_TOOL_SEARCH is unset/empty.
	// If user explicitly sets ENABLE_TOOL_SEARCH, assume their gateway supports it.
	if strings.TrimSpace(os.Getenv("ENABLE_TOOL_SEARCH")) == "" &&
		model.GetAPIProvider() == model.ProviderFirstParty &&
		!model.IsFirstPartyAnthropicBaseURL() {
		return false
	}
	return true
}

func IsToolSearchToolAvailable(tools []tool.Tool) bool {
	for _, t := range tools {
		if strings.EqualFold(strings.TrimSpace(t.Name()), "ToolSearch") {
			return true
		}
	}
	return false
}

// IsDeferredTool mirrors TS isDeferredTool: MCP tools plus any tool that opts in.
func IsDeferredTool(t tool.Tool) bool {
	if mt, ok := t.(interface{ IsMCPTool() bool }); ok && mt.IsMCPTool() {
		return true
	}
	if dt, ok := t.(interface{ IsDeferredTool() bool }); ok && dt.IsDeferredTool() {
		return true
	}
	return false
}

func deferredToolDescriptionChars(tools []tool.Tool) int {
	total := 0
	for _, t := range tools {
		if !IsDeferredTool(t) {
			continue
		}
		// Approximate what would hit the wire: name + prompt + schema.
		total += len(t.Name())
		total += len(t.Prompt())
		total += len(t.InputSchema())
	}
	return total
}

// IsEnabled is the definitive gate (TS: isToolSearchEnabled).
// It includes model support, ToolSearchTool availability, and threshold checks for auto mode.
func IsEnabled(modelName string, tools []tool.Tool) bool {
	if !IsEnabledOptimistic() {
		return false
	}
	if !ModelSupportsToolReference(modelName) {
		return false
	}
	if !IsToolSearchToolAvailable(tools) {
		return false
	}

	switch GetMode() {
	case ModeTST:
		return true
	case ModeTSTAuto:
		threshold := getAutoToolSearchCharThreshold(modelName)
		return deferredToolDescriptionChars(tools) >= threshold
	default:
		return false
	}
}
