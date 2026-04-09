package plugins

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"claude-code-running-go/src/services/mcp"
)

// LoadPluginMcpServers scans plugin roots for MCP server configs and returns a merged map.
// Precedence (per plugin): .mcp.json (lowest) -> plugin.json mcpServers (higher).
// Precedence (across plugins): later merge wins (last-wins).
func LoadPluginMcpServers(pluginRoots []string) map[string]mcp.ServerConfig {
	out := map[string]mcp.ServerConfig{}

	for _, root := range pluginRoots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		// Deterministic scan order.
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			pluginPath := filepath.Join(root, e.Name())
			mergeServers(out, loadMcpJSONFile(filepath.Join(pluginPath, ".mcp.json")))
			mergeServers(out, loadPluginManifestMcpServers(pluginPath))
		}
	}

	return out
}

func mergeServers(dst map[string]mcp.ServerConfig, src map[string]mcp.ServerConfig) {
	for k, v := range src {
		dst[k] = v
	}
}

func loadMcpJSONFile(path string) map[string]mcp.ServerConfig {
	if _, err := os.Stat(path); err != nil {
		return nil
	}
	cfg, err := mcp.LoadConfigFile(path)
	if err != nil {
		return nil
	}
	return cfg.McpServers
}

type pluginManifest struct {
	McpServers json.RawMessage `json:"mcpServers"`
}

func loadPluginManifestMcpServers(pluginPath string) map[string]mcp.ServerConfig {
	// TS checks both .claude-plugin/plugin.json and legacy plugin.json.
	candidates := []string{
		filepath.Join(pluginPath, ".claude-plugin", "plugin.json"),
		filepath.Join(pluginPath, "plugin.json"),
	}
	var mp pluginManifest
	found := false
	for _, p := range candidates {
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		if err := json.Unmarshal(b, &mp); err != nil {
			continue
		}
		if len(mp.McpServers) == 0 {
			return nil
		}
		found = true
		break
	}
	if !found {
		return nil
	}
	return parseMcpServersSpec(pluginPath, mp.McpServers)
}

func parseMcpServersSpec(pluginPath string, raw json.RawMessage) map[string]mcp.ServerConfig {
	// Spec may be: string (path), array (paths/inline), or object (inline server map).
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		return loadMcpServersFromSpecString(pluginPath, asString)
	}
	var asArray []json.RawMessage
	if err := json.Unmarshal(raw, &asArray); err == nil {
		out := map[string]mcp.ServerConfig{}
		for _, item := range asArray {
			mergeServers(out, parseMcpServersSpec(pluginPath, item))
		}
		return out
	}
	// object
	var asObj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asObj); err == nil {
		out := map[string]mcp.ServerConfig{}
		for name, cfgRaw := range asObj {
			var sc mcp.ServerConfig
			_ = json.Unmarshal(cfgRaw, &sc)
			if strings.TrimSpace(sc.Command) != "" {
				out[name] = sc
			}
		}
		return out
	}
	return nil
}

func loadMcpServersFromSpecString(pluginPath string, spec string) map[string]mcp.ServerConfig {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return nil
	}
	// MCPB/DXT: load bundle (local path or URL).
	if strings.HasSuffix(spec, ".mcpb") || strings.HasSuffix(spec, ".dxt") {
		servers, err := LoadMcpbSpec(context.Background(), pluginPath, spec)
		if err == nil {
			return servers
		}
		return nil
	}
	// Treat as relative JSON file path within plugin.
	p := spec
	// TS requires relative paths like ./foo.json; accept both.
	p = strings.TrimPrefix(p, "./")
	p = filepath.Join(pluginPath, p)
	return loadMcpJSONFile(p)
}
