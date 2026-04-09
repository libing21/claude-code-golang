package memdir

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Minimal Go port for non-UI parity. TS has a much richer gating chain.

func envTruthy(key string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func IsAutoMemoryEnabled() bool {
	// Match the TS "disable auto memory" env.
	if envTruthy("CLAUDE_CODE_DISABLE_AUTO_MEMORY") {
		return false
	}
	// TS SIMPLE disables memory. Keep parity.
	if envTruthy("CLAUDE_CODE_SIMPLE") {
		return false
	}
	return true
}

func getClaudeConfigHomeDir() string {
	// TS uses getClaudeConfigHomeDir(); approximate for now.
	if v := strings.TrimSpace(os.Getenv("CLAUDE_CONFIG_DIR")); v != "" {
		return v
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".claude")
	}
	return ".claude"
}

func GetMemoryBaseDir() string {
	if v := strings.TrimSpace(os.Getenv("CLAUDE_CODE_REMOTE_MEMORY_DIR")); v != "" {
		return v
	}
	return getClaudeConfigHomeDir()
}

func sanitizeProjectSlug(absPath string) string {
	absPath = strings.TrimSpace(absPath)
	if absPath == "" {
		return "unknown"
	}
	absPath = filepath.Clean(absPath)
	// Replace separators and other unsafe chars.
	re := regexp.MustCompile(`[^a-zA-Z0-9._-]+`)
	s := re.ReplaceAllString(absPath, "_")
	s = strings.Trim(s, "_")
	if len(s) > 80 {
		s = s[len(s)-80:]
	}
	if s == "" {
		return "unknown"
	}
	return s
}

func GetAutoMemPath(cwd string) string {
	base := GetMemoryBaseDir()
	slug := sanitizeProjectSlug(cwd)
	return filepath.Join(base, "projects", slug, "memory") + string(os.PathSeparator)
}

func GetTeamMemPath(cwd string) string {
	return filepath.Join(GetAutoMemPath(cwd), "team") + string(os.PathSeparator)
}

