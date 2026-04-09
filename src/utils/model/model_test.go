package model

import "testing"

func TestGetMainLoopModelUsesAnthropicModel(t *testing.T) {
	t.Setenv("ANTHROPIC_MODEL", "qwen3.5-flash")
	t.Setenv("ANTHROPIC_DEFAULT_SONNET_MODEL", "fallback-sonnet")
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "")

	got := GetMainLoopModel("")
	if got != "qwen3.5-flash" {
		t.Fatalf("expected ANTHROPIC_MODEL to win, got %q", got)
	}
}

func TestGetMainLoopModelFallsBackToDefaultSonnet(t *testing.T) {
	t.Setenv("ANTHROPIC_MODEL", "")
	t.Setenv("ANTHROPIC_DEFAULT_SONNET_MODEL", "qwen3.5-flash")
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "")

	got := GetMainLoopModel("")
	if got != "qwen3.5-flash" {
		t.Fatalf("expected default sonnet model, got %q", got)
	}
}

func TestParseAliasSonnet(t *testing.T) {
	t.Setenv("ANTHROPIC_DEFAULT_SONNET_MODEL", "qwen3.5-flash")
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "")

	got := ParseUserSpecifiedModel("sonnet")
	if got != "qwen3.5-flash" {
		t.Fatalf("expected alias sonnet to resolve to qwen3.5-flash, got %q", got)
	}
}

func TestParseAliasHaiku(t *testing.T) {
	t.Setenv("ANTHROPIC_DEFAULT_HAIKU_MODEL", "qwen3.5-flash")

	got := ParseUserSpecifiedModel("haiku")
	if got != "qwen3.5-flash" {
		t.Fatalf("expected alias haiku to resolve to qwen3.5-flash, got %q", got)
	}
}

func TestExplicitModelOverrideWins(t *testing.T) {
	t.Setenv("ANTHROPIC_MODEL", "from-env")

	got := GetMainLoopModel("qwen-plus-latest")
	if got != "qwen-plus-latest" {
		t.Fatalf("expected explicit model to win, got %q", got)
	}
}

func TestGetDefaultSonnetModelUses3PDefaultWhenProviderIsBedrock(t *testing.T) {
	t.Setenv("ANTHROPIC_DEFAULT_SONNET_MODEL", "")
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "1")
	t.Setenv("CLAUDE_CODE_USE_VERTEX", "")
	t.Setenv("CLAUDE_CODE_USE_FOUNDRY", "")

	got := GetDefaultSonnetModel()
	if got != GetModelStrings().Sonnet45 {
		t.Fatalf("expected 3P default sonnet45, got %q", got)
	}
}

func TestGetDefaultSonnetModelUses1PDefaultWhenNo3PProvider(t *testing.T) {
	t.Setenv("ANTHROPIC_DEFAULT_SONNET_MODEL", "")
	t.Setenv("CLAUDE_CODE_USE_BEDROCK", "")
	t.Setenv("CLAUDE_CODE_USE_VERTEX", "")
	t.Setenv("CLAUDE_CODE_USE_FOUNDRY", "")

	got := GetDefaultSonnetModel()
	if got != GetModelStrings().Sonnet46 {
		t.Fatalf("expected 1P default sonnet46, got %q", got)
	}
}

func TestIsFirstPartyAnthropicBaseURL(t *testing.T) {
	t.Setenv("ANTHROPIC_BASE_URL", "https://api.anthropic.com")
	if !IsFirstPartyAnthropicBaseURL() {
		t.Fatalf("expected api.anthropic.com to be first-party")
	}
}
