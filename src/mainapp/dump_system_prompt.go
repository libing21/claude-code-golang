package mainapp

import (
	"strings"

	"claude-code-running-go/src/constants/prompts"
	"claude-code-running-go/src/query"
	"claude-code-running-go/src/utils/model"
)

func DumpSystemPrompt(cfg Config) (string, error) {
	cfg.Model = model.GetMainLoopModel(cfg.Model)
	cfg.Model = model.GetRuntimeMainLoopModel(cfg.PermissionMode, cfg.Model, false)

	enabledTools := query.GetDefaultToolNamesForPrompt(cfg.SkillDirs)
	systemPromptParts, err := prompts.BuildDefaultSystemPrompt(cfg.Model, enabledTools)
	if err != nil {
		return "", err
	}

	effective := prompts.BuildEffectiveSystemPrompt(prompts.EffectivePromptInput{
		Default:  systemPromptParts,
		Override: cfg.SystemPromptOverride,
		Append:   cfg.AppendSystemPrompt,
	})
	return strings.Join(effective, "\n\n"), nil
}
