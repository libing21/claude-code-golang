package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Minimal secure/non-secure storage split inspired by TS mcpbHandler:
// - Non-sensitive: ~/.claude-go/plugin_configs.json
// - Sensitive:     ~/.claude-go/plugin_secrets.json (0600)

type pluginConfigFile struct {
	Plugins map[string]map[string]map[string]any `json:"plugins"` // pluginID -> serverName -> key -> value
}

func configHome() string {
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".claude-go")
	}
	// Fallback: local dir
	return ".claude-go"
}

func readJSONFile(path string, out any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}

func writeJSONFile(path string, v any, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	// Write then chmod to enforce perms.
	if err := os.WriteFile(path, b, perm); err != nil {
		return err
	}
	return os.Chmod(path, perm)
}

func loadPluginConfig(pluginID string, serverName string) (map[string]any, error) {
	path := filepath.Join(configHome(), "plugin_configs.json")
	var cfg pluginConfigFile
	if err := readJSONFile(path, &cfg); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if cfg.Plugins == nil {
		return nil, nil
	}
	s, ok := cfg.Plugins[pluginID]
	if !ok {
		return nil, nil
	}
	v, ok := s[serverName]
	if !ok {
		return nil, nil
	}
	return v, nil
}

func loadPluginSecret(pluginID string, serverName string) (map[string]any, error) {
	path := filepath.Join(configHome(), "plugin_secrets.json")
	var cfg pluginConfigFile
	if err := readJSONFile(path, &cfg); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if cfg.Plugins == nil {
		return nil, nil
	}
	s, ok := cfg.Plugins[pluginID]
	if !ok {
		return nil, nil
	}
	v, ok := s[serverName]
	if !ok {
		return nil, nil
	}
	return v, nil
}
