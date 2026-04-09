package model

import (
	"os"
	"strings"
)

const (
	AliasSonnet   = "sonnet"
	AliasOpus     = "opus"
	AliasHaiku    = "haiku"
	AliasBest     = "best"
	AliasOpusPlan = "opusplan"
)

func GetDefaultOpusModel() string {
	if v := strings.TrimSpace(os.Getenv("ANTHROPIC_DEFAULT_OPUS_MODEL")); v != "" {
		return v
	}
	return GetModelStrings().Opus46
}

func GetDefaultSonnetModel() string {
	if v := strings.TrimSpace(os.Getenv("ANTHROPIC_DEFAULT_SONNET_MODEL")); v != "" {
		return v
	}
	// Match TS external behavior: 3P providers default Sonnet to 4.5.
	if GetAPIProvider() != ProviderFirstParty {
		return GetModelStrings().Sonnet45
	}
	return GetModelStrings().Sonnet46
}

func GetDefaultHaikuModel() string {
	if v := strings.TrimSpace(os.Getenv("ANTHROPIC_DEFAULT_HAIKU_MODEL")); v != "" {
		return v
	}
	return GetModelStrings().Haiku45
}

func GetBestModel() string {
	return GetDefaultOpusModel()
}

// GetMainLoopModel mirrors the relevant external TS behavior:
// 1. explicit model (CLI override / session override)
// 2. ANTHROPIC_MODEL from env
// 3. default main loop model (external users -> Sonnet default)
func GetMainLoopModel(explicit string) string {
	if v := strings.TrimSpace(explicit); v != "" {
		return ParseUserSpecifiedModel(v)
	}
	if v := strings.TrimSpace(os.Getenv("ANTHROPIC_MODEL")); v != "" {
		return ParseUserSpecifiedModel(v)
	}
	return GetDefaultMainLoopModel()
}

// For the Go port we currently follow the external-user path from TS:
// default main model => default Sonnet model unless explicitly configured.
func GetDefaultMainLoopModel() string {
	return ParseUserSpecifiedModel(GetDefaultSonnetModel())
}

// ParseUserSpecifiedModel mirrors the TS alias handling closely enough for the Go port.
// It preserves custom model IDs exactly, while resolving known aliases through env defaults.
func ParseUserSpecifiedModel(modelInput string) string {
	modelInputTrimmed := strings.TrimSpace(modelInput)
	if modelInputTrimmed == "" {
		return GetDefaultMainLoopModel()
	}

	normalized := strings.ToLower(modelInputTrimmed)
	has1mTag := strings.HasSuffix(normalized, "[1m]")
	modelString := normalized
	if has1mTag {
		modelString = strings.TrimSpace(strings.TrimSuffix(modelString, "[1m]"))
	}

	switch modelString {
	case AliasOpusPlan:
		// TS currently resolves opusplan -> default sonnet, with special plan-mode behavior elsewhere.
		if has1mTag {
			return GetDefaultSonnetModel() + "[1m]"
		}
		return GetDefaultSonnetModel()
	case AliasSonnet:
		if has1mTag {
			return GetDefaultSonnetModel() + "[1m]"
		}
		return GetDefaultSonnetModel()
	case AliasHaiku:
		if has1mTag {
			return GetDefaultHaikuModel() + "[1m]"
		}
		return GetDefaultHaikuModel()
	case AliasOpus:
		if has1mTag {
			return GetDefaultOpusModel() + "[1m]"
		}
		return GetDefaultOpusModel()
	case AliasBest:
		return GetBestModel()
	default:
		// Preserve original case for custom model IDs, only normalizing [1m] suffix.
		if has1mTag {
			base := strings.TrimSpace(modelInputTrimmed[:len(modelInputTrimmed)-4])
			return base + "[1m]"
		}
		return modelInputTrimmed
	}
}

// Runtime model selection mirrors the subset of TS behavior that matters for print mode.
// We keep permissionMode as string for now to avoid pulling the full permissions model in.
func GetRuntimeMainLoopModel(permissionMode string, mainLoopModel string, exceeds200kTokens bool) string {
	userSpecified := strings.TrimSpace(os.Getenv("ANTHROPIC_MODEL"))
	if userSpecified == AliasOpusPlan && permissionMode == "plan" && !exceeds200kTokens {
		return GetDefaultOpusModel()
	}
	if userSpecified == AliasHaiku && permissionMode == "plan" {
		return GetDefaultSonnetModel()
	}
	return mainLoopModel
}
