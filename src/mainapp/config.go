package mainapp

import "os"

// Config is a minimal subset for phase-1 (--print).
// Mirrors env vars used by the TS version (see README.md).
type Config struct {
	BaseURL              string
	Model                string // raw user/env setting, resolved later via utils/model
	APIKey               string
	AuthToken            string
	PermissionMode       string
	Debug                bool
	MessagesDumpDir      string
	AllowedTools         []string
	DisallowedTools      []string
	MCPConfigPath        string
	PluginDirs           []string
	SkillDirs            []string
	SystemPromptOverride string
	AppendSystemPrompt   string
}

func ConfigFromEnv() Config {
	return Config{
		BaseURL:              os.Getenv("ANTHROPIC_BASE_URL"),
		Model:                os.Getenv("ANTHROPIC_MODEL"),
		APIKey:               os.Getenv("ANTHROPIC_API_KEY"),
		AuthToken:            os.Getenv("ANTHROPIC_AUTH_TOKEN"),
		PermissionMode:       "default",
		SystemPromptOverride: "",
		AppendSystemPrompt:   "",
	}
}
