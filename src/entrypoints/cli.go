package entrypoints

import (
	"context"
	"errors"
	"fmt"
	"os"

	"claude-code-running-go/src/mainapp"
	"claude-code-running-go/src/utils/env"
)

// CLIOptions is intentionally kept close to TS entrypoint flags.
// TS reference: src/entrypoints/cli.tsx -> src/main.tsx
type CLIOptions struct {
	PrintMode          bool
	DumpSystemPrompt   bool
	Debug              bool
	MessagesDumpDir    string
	Prompt             string
	Model              string
	BaseURL            string
	APIKey             string
	AuthToken          string
	PermissionMode     string
	AllowedTools       []string
	DisallowedTools    []string
	MCPConfigPath      string
	PluginDirs         []string
	SkillDirs          []string
	SystemPrompt       string
	AppendSystemPrompt string
}

func RunCLI(ctx context.Context, opts CLIOptions) (string, error) {
	// Best-effort load .env from likely workspace locations.
	_ = env.LoadDotEnv(
		".env",
		"claude-code-running-go/.env",
		"claude-code-runing/.env",
		"../claude-code-runing/.env",
	)

	if opts.Prompt == "" && !opts.DumpSystemPrompt {
		return "", errors.New("prompt is required")
	}

	// Allow CLI flags to override env.
	cfg := mainapp.ConfigFromEnv()
	if opts.Model != "" {
		cfg.Model = opts.Model
	}
	if opts.BaseURL != "" {
		cfg.BaseURL = opts.BaseURL
	}
	if opts.APIKey != "" {
		cfg.APIKey = opts.APIKey
	}
	if opts.AuthToken != "" {
		cfg.AuthToken = opts.AuthToken
	}
	if opts.SystemPrompt != "" {
		cfg.SystemPromptOverride = opts.SystemPrompt
	}
	if opts.AppendSystemPrompt != "" {
		cfg.AppendSystemPrompt = opts.AppendSystemPrompt
	}
	if opts.PermissionMode != "" {
		cfg.PermissionMode = opts.PermissionMode
	}
	cfg.Debug = opts.Debug
	cfg.MessagesDumpDir = opts.MessagesDumpDir
	cfg.AllowedTools = append(cfg.AllowedTools, opts.AllowedTools...)
	cfg.DisallowedTools = append(cfg.DisallowedTools, opts.DisallowedTools...)
	if opts.MCPConfigPath != "" {
		cfg.MCPConfigPath = opts.MCPConfigPath
	}
	cfg.PluginDirs = append(cfg.PluginDirs, opts.PluginDirs...)
	cfg.SkillDirs = append(cfg.SkillDirs, opts.SkillDirs...)

	if opts.DumpSystemPrompt {
		return mainapp.DumpSystemPrompt(cfg)
	}

	if cfg.BaseURL == "" {
		// Keep it explicit so debugging is straightforward.
		return "", fmt.Errorf("missing base url: set ANTHROPIC_BASE_URL or pass --base-url")
	}
	if cfg.APIKey == "" && cfg.AuthToken == "" {
		return "", fmt.Errorf("missing auth: set ANTHROPIC_API_KEY or ANTHROPIC_AUTH_TOKEN (or pass flags)")
	}

	// For phase-1 we only support --print (non-interactive).
	if !opts.PrintMode {
		fmt.Fprintln(os.Stderr, "TUI mode is not implemented in Go port yet. Use --print.")
		return "", fmt.Errorf("interactive mode not implemented")
	}

	return mainapp.RunPrint(ctx, cfg, opts.Prompt)
}
