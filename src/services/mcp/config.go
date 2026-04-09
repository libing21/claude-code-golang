package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func LoadConfigFile(path string) (ConfigFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return ConfigFile{}, err
	}
	var cfg ConfigFile
	if err := json.Unmarshal(b, &cfg); err != nil {
		return ConfigFile{}, fmt.Errorf("parse mcp config: %w", err)
	}
	if cfg.McpServers == nil {
		cfg.McpServers = map[string]ServerConfig{}
	}
	return cfg, nil
}

// FindDefaultConfigPaths matches common TS locations:
// - project: .mcp.json, .claude/.mcp.json
// - user: ~/.claude/.mcp.json
func FindDefaultConfigPaths() []string {
	var out []string
	out = append(out, ".mcp.json")
	out = append(out, filepath.Join(".claude", ".mcp.json"))
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		out = append(out, filepath.Join(home, ".claude", ".mcp.json"))
	}
	return out
}

func LoadFirstExistingConfig(paths []string) (ConfigFile, string, error) {
	var lastErr error
	for _, p := range paths {
		if p == "" {
			continue
		}
		if _, err := os.Stat(p); err != nil {
			lastErr = err
			continue
		}
		cfg, err := LoadConfigFile(p)
		if err != nil {
			return ConfigFile{}, p, err
		}
		return cfg, p, nil
	}
	if lastErr != nil {
		return ConfigFile{}, "", lastErr
	}
	return ConfigFile{}, "", fmt.Errorf("no mcp config found")
}

