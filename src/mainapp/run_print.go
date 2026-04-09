package mainapp

import (
	"context"
	"fmt"

	"claude-code-running-go/src/constants/prompts"
	"claude-code-running-go/src/query"
	"claude-code-running-go/src/services/api"
	"claude-code-running-go/src/utils/model"
)

// RunPrint runs a single non-interactive prompt and returns final text.
func RunPrint(ctx context.Context, cfg Config, userPrompt string) (string, error) {
	cfg.Model = model.GetMainLoopModel(cfg.Model)
	cfg.Model = model.GetRuntimeMainLoopModel(cfg.PermissionMode, cfg.Model, false)

	enabledTools := query.GetDefaultToolNamesForPrompt(cfg.SkillDirs)
	systemPromptParts, err := prompts.BuildDefaultSystemPrompt(cfg.Model, enabledTools)
	if err != nil {
		return "", err
	}

	effective := prompts.BuildEffectiveSystemPrompt(prompts.EffectivePromptInput{
		Default:              systemPromptParts,
		Override:             cfg.SystemPromptOverride,
		Append:               cfg.AppendSystemPrompt,
	})

	client := api.NewClient(api.ClientConfig{
		BaseURL:   cfg.BaseURL,
		APIKey:    cfg.APIKey,
		AuthToken: cfg.AuthToken,
		Model:     cfg.Model,
	})

	out, err := query.RunOnceWithToolLoop(
		ctx,
		client,
		effective,
		userPrompt,
		cfg.PermissionMode,
		cfg.AllowedTools,
		cfg.DisallowedTools,
		cfg.MCPConfigPath,
		cfg.PluginDirs,
		cfg.SkillDirs,
		cfg.Debug,
		cfg.MessagesDumpDir,
	)
	if err != nil {
		return "", err
	}
	// For now, return text only.
	if out.Text == "" {
		return "", fmt.Errorf("empty response")
	}
	return out.Text, nil
}
