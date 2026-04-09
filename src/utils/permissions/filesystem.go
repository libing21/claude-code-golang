package permissions

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"claude-code-running-go/src/tool"
)

// Go port (subset) of TS src/utils/permissions/filesystem.ts.
// Focus: path normalization, case-safe comparisons, dangerous file/dir guards,
// and .claude/skills scope detection.

var (
	originalCwd string
	homeDir     string
)

func init() {
	originalCwd, _ = os.Getwd()
	homeDir, _ = os.UserHomeDir()
}

var dangerousFiles = map[string]struct{}{
	".gitconfig":  {},
	".gitmodules": {},
	".bashrc":     {},
	".bash_profile": {},
	".zshrc":        {},
	".zprofile":     {},
	".profile":      {},
	".ripgreprc":     {},
	".mcp.json":      {},
	".claude.json":   {},
}

var dangerousDirs = map[string]struct{}{
	".git":    {},
	".vscode": {},
	".idea":   {},
	".claude": {},
}

func normalizeCaseForComparison(p string) string {
	// TS always lowercases regardless of platform for consistent security.
	// This prevents bypasses on macOS/Windows case-insensitive FS.
	return strings.ToLower(p)
}

func splitPathListEnv(key string) []string {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, string(os.PathListSeparator))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func expandPath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return ""
	}
	if strings.HasPrefix(p, "~"+string(os.PathSeparator)) || p == "~" {
		if homeDir != "" {
			if p == "~" {
				p = homeDir
			} else {
				p = filepath.Join(homeDir, p[2:])
			}
		}
	}
	if !filepath.IsAbs(p) {
		base := originalCwd
		if base == "" {
			base, _ = os.Getwd()
		}
		p = filepath.Join(base, p)
	}
	return filepath.Clean(p)
}

func containsPathTraversal(p string) bool {
	// Conservative: reject any ".." path segment.
	p = filepath.Clean(p)
	parts := strings.Split(p, string(os.PathSeparator))
	for _, s := range parts {
		if s == ".." {
			return true
		}
	}
	// Also check POSIX separators for mixed inputs.
	if os.PathSeparator != '/' {
		parts = strings.Split(strings.ReplaceAll(p, string(os.PathSeparator), "/"), "/")
		for _, s := range parts {
			if s == ".." {
				return true
			}
		}
	}
	return false
}

func pathInDir(absPath string, absDir string) bool {
	ap := normalizeCaseForComparison(absPath)
	ad := normalizeCaseForComparison(absDir)

	// Ensure dir ends with separator.
	sep := string(os.PathSeparator)
	if !strings.HasSuffix(ad, sep) {
		ad += sep
	}
	return strings.HasPrefix(ap, ad)
}

func isDangerousPath(absPath string) bool {
	base := normalizeCaseForComparison(filepath.Base(absPath))
	if _, ok := dangerousFiles[base]; ok {
		return true
	}
	// Check each segment for dangerous dirs.
	cur := absPath
	for cur != "" && cur != string(os.PathSeparator) {
		b := normalizeCaseForComparison(filepath.Base(cur))
		if _, ok := dangerousDirs[b]; ok {
			return true
		}
		next := filepath.Dir(cur)
		if next == cur {
			break
		}
		cur = next
	}
	return false
}

// getClaudeSkillScope returns skill name if the path is inside .claude/skills/<name>/...
// (project-local or global ~/.claude/skills). Mirrors TS behavior at a high level.
func getClaudeSkillScope(filePath string) (skillName string, ok bool) {
	abs := expandPath(filePath)
	absLower := normalizeCaseForComparison(abs)

	type base struct {
		dir string
	}
	bases := []base{
		{dir: expandPath(filepath.Join(originalCwd, ".claude", "skills"))},
	}
	if homeDir != "" {
		bases = append(bases, base{dir: expandPath(filepath.Join(homeDir, ".claude", "skills"))})
	}
	for _, b := range bases {
		dirLower := normalizeCaseForComparison(b.dir)
		sep := string(os.PathSeparator)
		if strings.HasPrefix(absLower, dirLower+strings.ToLower(sep)) {
			rest := abs[len(b.dir)+len(sep):]
			rest = strings.ReplaceAll(rest, "\\", "/")
			cut := strings.Index(rest, "/")
			if cut <= 0 {
				return "", false
			}
			name := rest[:cut]
			if name == "" || name == "." || strings.Contains(name, "..") {
				return "", false
			}
			if strings.ContainsAny(name, "*?[]") {
				return "", false
			}
			return name, true
		}
	}
	return "", false
}

type PathPolicy struct {
	ProjectRoot    string
	AllowedRoots   []string
	ScratchpadRoot string
}

func DefaultPathPolicy() PathPolicy {
	root := originalCwd
	if root == "" {
		root, _ = os.Getwd()
	}
	allowed := splitPathListEnv("CLAUDE_GO_ALLOWED_DIRS")
	scratch := strings.TrimSpace(os.Getenv("CLAUDE_GO_SCRATCHPAD_DIR"))
	if scratch == "" {
		scratch = ".claude-go/scratchpad"
	}
	return PathPolicy{
		ProjectRoot:    expandPath(root),
		AllowedRoots:   allowed,
		ScratchpadRoot: expandPath(scratch),
	}
}

type PathOp string

const (
	OpRead  PathOp = "read"
	OpGlob  PathOp = "glob"
	OpGrep  PathOp = "grep"
	OpEdit  PathOp = "edit"
	OpWrite PathOp = "write"
	OpBash  PathOp = "bash"
)

// CheckPath returns a tool-style permission decision and normalized absolute path.
// This check is tool-specific and should run even in bypass mode to prevent obvious escapes.
func CheckPath(policy PathPolicy, op PathOp, rawPath string) (tool.PermissionDecision, string) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return tool.PermissionDecision{
			Behavior: tool.PermissionBehaviorDeny,
			Reason:   "empty path",
			CheckTag: "fs.empty",
			CheckID:  3000,
		}, ""
	}
	if containsPathTraversal(rawPath) {
		return tool.PermissionDecision{
			Behavior: tool.PermissionBehaviorDeny,
			Reason:   "path traversal is not allowed",
			CheckTag: "fs.traversal",
			CheckID:  3001,
		}, ""
	}

	abs := expandPath(rawPath)

	// Case-insensitive guard: normalize both sides for compare (TS behavior).
	inProject := pathInDir(abs, policy.ProjectRoot) || normalizeCaseForComparison(abs) == normalizeCaseForComparison(policy.ProjectRoot)
	inScratch := policy.ScratchpadRoot != "" && (pathInDir(abs, policy.ScratchpadRoot) || normalizeCaseForComparison(abs) == normalizeCaseForComparison(policy.ScratchpadRoot))
	inAllowed := false
	for _, r := range policy.AllowedRoots {
		if r == "" {
			continue
		}
		ar := expandPath(r)
		if pathInDir(abs, ar) || normalizeCaseForComparison(abs) == normalizeCaseForComparison(ar) {
			inAllowed = true
			break
		}
	}

	// Dangerous paths: ask for read, ask/deny for writes.
	if isDangerousPath(abs) {
		// Editing dangerous config should always prompt (matches TS spirit).
		if op == OpEdit || op == OpWrite || op == OpBash {
			skill, ok := getClaudeSkillScope(abs)
			if ok {
				return tool.PermissionDecision{
					Behavior: tool.PermissionBehaviorAsk,
					Reason:   fmt.Sprintf("path is within .claude/skills/%s; confirmation required", skill),
					CheckTag: "fs.claude_skill",
					CheckID:  3102,
				}, abs
			}
			return tool.PermissionDecision{
				Behavior: tool.PermissionBehaviorAsk,
				Reason:   "path is within a protected file/directory (.git/.claude/etc); confirmation required",
				CheckTag: "fs.dangerous",
				CheckID:  3101,
			}, abs
		}
		// Read-only: ask but do not deny by default.
		return tool.PermissionDecision{
			Behavior: tool.PermissionBehaviorAsk,
			Reason:   "reading protected file/directory; confirmation required",
			CheckTag: "fs.dangerous_read",
			CheckID:  3100,
		}, abs
	}

	if inProject || inScratch || inAllowed {
		return tool.PermissionDecision{
			Behavior: tool.PermissionBehaviorAllow,
			Reason:   "path allowed",
			CheckTag: "fs.allowed",
			CheckID:  3200,
		}, abs
	}

	// Outside project roots: allow read-only with ask (data exfil risk), and ask for writes.
	switch op {
	case OpRead, OpGlob, OpGrep:
		return tool.PermissionDecision{
			Behavior: tool.PermissionBehaviorAsk,
			Reason:   "path is outside project; confirmation required",
			CheckTag: "fs.outside_project_read",
			CheckID:  3300,
		}, abs
	default:
		return tool.PermissionDecision{
			Behavior: tool.PermissionBehaviorAsk,
			Reason:   "path is outside project; confirmation required",
			CheckTag: "fs.outside_project_write",
			CheckID:  3301,
		}, abs
	}
}

// For callers that want to mirror TS's "always normalize to lowercase" approach
// across platforms, we already do so above; this function exists to avoid unused imports.
func _platform() string { return runtime.GOOS }

