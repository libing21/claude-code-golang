package skill

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type Definition struct {
	Name            string
	Description     string
	Model           string
	PermissionMode  string
	MaxTurns        int
	Tools           []string
	DisallowedTools []string
	Body            string
	Dir             string
	Entry           string
	Kind            string // "markdown" or "script"
}

func configuredRoots(extra []string) []string {
	roots := make([]string, 0, 4)
	if v := strings.TrimSpace(os.Getenv("CLAUDE_GO_SKILLS_DIR")); v != "" {
		roots = append(roots, v)
	}
	roots = append(roots, extra...)
	return roots
}

func ResolveSkill(extraRoots []string, name string) (Definition, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Definition{}, false
	}
	for _, dir := range configuredRoots(extraRoots) {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			continue
		}
		skillDir := filepath.Join(dir, name)
		skillFile := filepath.Join(skillDir, "SKILL.md")
		if b, err := os.ReadFile(skillFile); err == nil {
			def := parseSkillMarkdown(name, skillDir, string(b))
			def.Entry = skillFile
			def.Kind = "markdown"
			return def, true
		}
		runFile := filepath.Join(skillDir, "run.sh")
		if _, err := os.Stat(runFile); err == nil {
			return Definition{Name: name, Dir: skillDir, Entry: runFile, Kind: "script"}, true
		}
		if _, err := os.Stat(skillDir); err == nil {
			info, err := os.Stat(skillDir)
			if err == nil && !info.IsDir() {
				return Definition{Name: name, Dir: dir, Entry: skillDir, Kind: "script"}, true
			}
		}
	}
	return Definition{}, false
}

func ListSkills(extraRoots []string) []Definition {
	out := make([]Definition, 0, 32)
	seen := map[string]struct{}{}
	for _, r := range configuredRoots(extraRoots) {
		r = strings.TrimSpace(r)
		if r == "" {
			continue
		}
		entries, err := os.ReadDir(r)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			if def, ok := ResolveSkill([]string{r}, name); ok {
				seen[name] = struct{}{}
				out = append(out, def)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func parseSkillMarkdown(defaultName, dir, markdown string) Definition {
	meta, body := parseFrontmatter(markdown)
	def := Definition{
		Name: defaultName,
		Dir:  dir,
		Body: strings.TrimSpace(body),
	}
	if v := strings.TrimSpace(asString(meta["name"])); v != "" {
		def.Name = v
	}
	def.Description = strings.TrimSpace(asString(meta["description"]))
	def.Model = strings.TrimSpace(asString(meta["model"]))
	def.PermissionMode = strings.TrimSpace(asString(meta["permissionMode"]))
	def.MaxTurns = asInt(meta["maxTurns"])
	def.Tools = asStringSlice(meta["tools"])
	def.DisallowedTools = asStringSlice(meta["disallowedTools"])
	return def
}

func parseFrontmatter(markdown string) (map[string]any, string) {
	sc := bufio.NewScanner(strings.NewReader(markdown))
	if !sc.Scan() || strings.TrimSpace(sc.Text()) != "---" {
		return map[string]any{}, markdown
	}
	meta := map[string]any{}
	var currentListKey string
	var consumed int
	consumed += len(sc.Text()) + 1
	for sc.Scan() {
		line := sc.Text()
		consumed += len(line) + 1
		trim := strings.TrimSpace(line)
		if trim == "---" {
			if consumed > len(markdown) {
				consumed = len(markdown)
			}
			return meta, markdown[consumed:]
		}
		if strings.HasPrefix(trim, "- ") && currentListKey != "" {
			cur, _ := meta[currentListKey].([]string)
			meta[currentListKey] = append(cur, strings.Trim(strings.TrimPrefix(trim, "- "), `"'`))
			continue
		}
		currentListKey = ""
		col := strings.Index(trim, ":")
		if col <= 0 {
			continue
		}
		key := strings.TrimSpace(trim[:col])
		val := strings.TrimSpace(trim[col+1:])
		if val == "" {
			currentListKey = key
			meta[key] = []string{}
			continue
		}
		meta[key] = strings.Trim(val, `"'`)
	}
	return map[string]any{}, markdown
}

func asString(v any) string {
	s, _ := v.(string)
	return s
}

func asInt(v any) int {
	switch n := v.(type) {
	case int:
		return n
	case string:
		n = strings.TrimSpace(n)
		if n == "" {
			return 0
		}
		var out int
		for _, ch := range n {
			if ch < '0' || ch > '9' {
				return 0
			}
			out = out*10 + int(ch-'0')
		}
		return out
	default:
		return 0
	}
}

func asStringSlice(v any) []string {
	switch s := v.(type) {
	case []string:
		return append([]string{}, s...)
	default:
		return nil
	}
}

