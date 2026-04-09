package plugins

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"claude-code-running-go/src/services/mcp"
)

// This is a pragmatic Go port of the TS mcpbHandler/mcpPluginIntegration behavior.
// It focuses on:
// - Recognizing .mcpb/.dxt specs
// - Loading from local path or URL
// - Extracting to pluginPath/.mcpb-cache/<hash>/
// - Deriving a McpServerConfig from either manifest.json heuristics or embedded .mcp.json
//
// It implements a minimal user_config flow:
// - Reads manifest.user_config schema
// - Loads non-sensitive values from ~/.claude-go/plugin_configs.json
// - Loads sensitive values from ~/.claude-go/plugin_secrets.json (0600)
// - Substitutes ${user_config.KEY} into server env
// - Skips loading if required keys are missing

type mcpbLoadResult struct {
	ServerName string
	Config     mcp.ServerConfig
	Hash       string
	Extracted  string
}

const (
	maxZipFileCount        = 100000
	maxZipTotalUncompressed = 1024 * 1024 * 1024 // 1GB
	maxZipFileSize         = 512 * 1024 * 1024   // 512MB per file
	maxZipCompressionRatio = 50.0
)

func isMcpbSource(spec string) bool {
	return strings.HasSuffix(spec, ".mcpb") || strings.HasSuffix(spec, ".dxt")
}

func isURL(spec string) bool {
	return strings.HasPrefix(spec, "http://") || strings.HasPrefix(spec, "https://")
}

func sha256Short(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])[:16]
}

func readAllURL(ctx context.Context, u string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "claude-code-running-go/0.1")
	hc := &http.Client{Timeout: 60 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4*1024))
		return nil, fmt.Errorf("download failed: %s: %s", resp.Status, string(body))
	}
	return io.ReadAll(io.LimitReader(resp.Body, 128*1024*1024))
}

func safeZipPath(name string) bool {
	if name == "" {
		return false
	}
	// zip always uses '/' separators.
	if strings.HasPrefix(name, "/") || strings.HasPrefix(name, "\\") {
		return false
	}
	clean := filepath.Clean(filepath.FromSlash(name))
	// Clean can collapse to "." for weird entries; treat as invalid.
	if clean == "." {
		return false
	}
	// Block absolute paths after normalization.
	if filepath.IsAbs(clean) {
		return false
	}
	// Block parent traversal segments.
	parts := strings.Split(clean, string(os.PathSeparator))
	for _, p := range parts {
		if p == ".." {
			return false
		}
	}
	return true
}

func extractZipToDir(zipBytes []byte, dest string) error {
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return err
	}
	// Zip bomb / resource limits (Go analog of TS dxt/zip.ts).
	var fileCount int
	var totalUncompressed uint64
	var totalCompressed uint64

	for _, f := range zr.File {
		fileCount++
		if fileCount > maxZipFileCount {
			return fmt.Errorf("archive contains too many files: %d", fileCount)
		}
		if f.UncompressedSize64 > uint64(maxZipFileSize) {
			return fmt.Errorf("archive file too large: %s (%d bytes)", f.Name, f.UncompressedSize64)
		}
		totalUncompressed += f.UncompressedSize64
		totalCompressed += f.CompressedSize64
		if totalUncompressed > uint64(maxZipTotalUncompressed) {
			return fmt.Errorf("archive total size too large: %d bytes", totalUncompressed)
		}
		// Defensive: avoid divide by zero.
		if totalCompressed > 0 {
			ratio := float64(totalUncompressed) / float64(totalCompressed)
			if ratio > maxZipCompressionRatio {
				return fmt.Errorf("suspicious compression ratio: %.1f:1", ratio)
			}
		}
	}

	for _, f := range zr.File {
		if !safeZipPath(f.Name) {
			return fmt.Errorf("unsafe path in archive: %q", f.Name)
		}
		target := filepath.Join(dest, filepath.FromSlash(f.Name))
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		b, err := io.ReadAll(io.LimitReader(rc, int64(maxZipFileSize)))
		if err != nil {
			return err
		}
		mode := f.Mode()
		// Default to 0644 but preserve executable bits when present.
		perm := os.FileMode(0o644)
		if mode&0o111 != 0 {
			perm = 0o755
		}
		if err := os.WriteFile(target, b, perm); err != nil {
			return err
		}
	}
	return nil
}

func findFileInZip(zipBytes []byte, want string) ([]byte, bool) {
	zr, err := zip.NewReader(bytes.NewReader(zipBytes), int64(len(zipBytes)))
	if err != nil {
		return nil, false
	}
	for _, f := range zr.File {
		if strings.EqualFold(filepath.Base(f.Name), want) {
			rc, err := f.Open()
			if err != nil {
				continue
			}
			defer rc.Close()
			b, err := io.ReadAll(io.LimitReader(rc, 8*1024*1024))
			if err != nil {
				continue
			}
			return b, true
		}
	}
	return nil, false
}

type manifestUserConfigOption struct {
	Sensitive bool `json:"sensitive,omitempty"`
	Required  bool `json:"required,omitempty"`
}

func parseUserConfigSchema(m any) map[string]manifestUserConfigOption {
	out := map[string]manifestUserConfigOption{}
	obj, ok := m.(map[string]any)
	if !ok {
		return out
	}
	raw, ok := obj["user_config"]
	if !ok {
		raw, ok = obj["userConfig"]
	}
	if !ok {
		return out
	}
	mm, ok := raw.(map[string]any)
	if !ok {
		return out
	}
	for k, v := range mm {
		b, _ := json.Marshal(v)
		var opt manifestUserConfigOption
		_ = json.Unmarshal(b, &opt)
		out[k] = opt
	}
	return out
}

func deriveServerFromManifest(extractedPath string, manifest any) (string, mcp.ServerConfig, map[string]manifestUserConfigOption, bool) {
	// Heuristics: try to find an object containing "command" and optional "args"/"env".
	m, ok := manifest.(map[string]any)
	if !ok {
		return "", mcp.ServerConfig{}, nil, false
	}
	name, _ := m["name"].(string)
	userSchema := parseUserConfigSchema(m)

	// Common nesting keys seen in DXT-like manifests.
	candidates := []string{"mcpServer", "mcp_server", "server", "mcp"}
	for _, k := range candidates {
		if v, ok := m[k]; ok {
			if sc, ok := parseServerConfig(extractedPath, v); ok {
				return nameOrFallback(name, extractedPath), sc, userSchema, true
			}
		}
	}

	// Sometimes command sits at top-level.
	if sc, ok := parseServerConfig(extractedPath, m); ok {
		return nameOrFallback(name, extractedPath), sc, userSchema, true
	}

	return "", mcp.ServerConfig{}, nil, false
}

func nameOrFallback(name string, extractedPath string) string {
	name = strings.TrimSpace(name)
	if name != "" {
		return name
	}
	return filepath.Base(extractedPath)
}

func parseServerConfig(extractedPath string, v any) (mcp.ServerConfig, bool) {
	obj, ok := v.(map[string]any)
	if !ok {
		return mcp.ServerConfig{}, false
	}
	cmd, _ := obj["command"].(string)
	if strings.TrimSpace(cmd) == "" {
		return mcp.ServerConfig{}, false
	}
	args := []string{}
	if rawArgs, ok := obj["args"].([]any); ok {
		for _, a := range rawArgs {
			if s, ok := a.(string); ok {
				args = append(args, s)
			}
		}
	}
	env := map[string]string{}
	if rawEnv, ok := obj["env"].(map[string]any); ok {
		for k, vv := range rawEnv {
			if s, ok := vv.(string); ok {
				env[k] = s
			}
		}
	}

	// If command is relative, resolve within extracted bundle.
	if !filepath.IsAbs(cmd) && !strings.Contains(cmd, "/") && !strings.Contains(cmd, "\\") {
		// Keep as-is: rely on PATH.
	} else if !filepath.IsAbs(cmd) {
		cmd = filepath.Join(extractedPath, filepath.FromSlash(cmd))
		cmd = filepath.Clean(cmd)
	}

	return mcp.ServerConfig{Command: cmd, Args: args, Env: env}, true
}

func loadMcpServersFromBundle(extractedPath string, zipBytes []byte) (map[string]mcp.ServerConfig, error) {
	// Prefer manifest-derived single-server config, matching TS "serverName = manifest.name".
	if b, ok := findFileInZip(zipBytes, "manifest.json"); ok {
		var manifest any
		if err := json.Unmarshal(b, &manifest); err == nil {
			if serverName, sc, _, ok := deriveServerFromManifest(extractedPath, manifest); ok {
				return map[string]mcp.ServerConfig{serverName: sc}, nil
			}
		}
	}

	// Fallback: if bundle contains an .mcp.json, load it.
	if b, ok := findFileInZip(zipBytes, ".mcp.json"); ok {
		var cfg mcp.ConfigFile
		if err := json.Unmarshal(b, &cfg); err == nil && len(cfg.McpServers) > 0 {
			return cfg.McpServers, nil
		}
		// Some bundles may store raw map without mcpServers wrapper.
		var raw map[string]mcp.ServerConfig
		if err := json.Unmarshal(b, &raw); err == nil && len(raw) > 0 {
			return raw, nil
		}
	}

	return nil, fmt.Errorf("bundle did not contain a usable manifest.json or .mcp.json")
}

func applyUserConfig(pluginID string, serverName string, cfg mcp.ServerConfig, schema map[string]manifestUserConfigOption) (mcp.ServerConfig, error) {
	if len(schema) == 0 {
		return cfg, nil
	}

	nonSensitive, _ := loadPluginConfig(pluginID, serverName)
	sensitive, _ := loadPluginSecret(pluginID, serverName)

	merged := map[string]any{}
	for k, v := range nonSensitive {
		merged[k] = v
	}
	for k, v := range sensitive {
		merged[k] = v
	}

	// Validate required keys.
	for k, opt := range schema {
		if opt.Required {
			if _, ok := merged[k]; !ok {
				return mcp.ServerConfig{}, fmt.Errorf("mcpb needs config: missing required user_config.%s", k)
			}
		}
	}

	// Substitute ${user_config.KEY} in env values.
	if cfg.Env == nil {
		return cfg, nil
	}
	for k, v := range cfg.Env {
		cfg.Env[k] = substituteUserConfig(v, merged)
	}
	return cfg, nil
}

func substituteUserConfig(s string, values map[string]any) string {
	// Minimal substitution: ${user_config.KEY} -> value
	for k, v := range values {
		needle := "${user_config." + k + "}"
		if strings.Contains(s, needle) {
			s = strings.ReplaceAll(s, needle, fmt.Sprintf("%v", v))
		}
	}
	return s
}

func LoadMcpbSpec(ctx context.Context, pluginPath string, spec string) (map[string]mcp.ServerConfig, error) {
	spec = strings.TrimSpace(spec)
	if !isMcpbSource(spec) {
		return nil, fmt.Errorf("not an mcpb/dxt spec")
	}

	var zipBytes []byte
	var err error
	if isURL(spec) {
		zipBytes, err = readAllURL(ctx, spec)
	} else {
		p := strings.TrimPrefix(spec, "./")
		p = filepath.Join(pluginPath, p)
		zipBytes, err = os.ReadFile(p)
	}
	if err != nil {
		return nil, err
	}

	hash := sha256Short(zipBytes)
	cacheDir := filepath.Join(pluginPath, ".mcpb-cache", hash)
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, err
	}

	// Extract best-effort; ok if already exists.
	_ = extractZipToDir(zipBytes, cacheDir)

	servers, err := loadMcpServersFromBundle(cacheDir, zipBytes)
	if err != nil {
		return nil, err
	}

	// If manifest has user_config, enforce it for manifest-derived single-server configs.
	if b, ok := findFileInZip(zipBytes, "manifest.json"); ok {
		var manifest any
		if err := json.Unmarshal(b, &manifest); err == nil {
			if serverName, sc, schema, ok := deriveServerFromManifest(cacheDir, manifest); ok && len(schema) > 0 {
				pluginID := filepath.Base(pluginPath)
				updated, err := applyUserConfig(pluginID, serverName, sc, schema)
				if err != nil {
					return nil, err
				}
				return map[string]mcp.ServerConfig{serverName: updated}, nil
			}
		}
	}

	return servers, nil
}
