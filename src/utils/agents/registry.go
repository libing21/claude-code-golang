package agents

import (
	"bufio"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Go port: minimal agent registry + markdown frontmatter parsing for .claude/agents.
// This is intentionally a subset of TS AgentTool/loadAgentsDir.ts — we focus on:
// - agentType (frontmatter.name)
// - whenToUse (frontmatter.description)
// - requiredMcpServers (frontmatter.requiredMcpServers)
//
// Hooks, mcpServers inline configs, skills preload, etc. can be added later.

type Definition struct {
	AgentType          string
	WhenToUse          string
	Tools              []string
	DisallowedTools    []string
	Model              string
	PermissionMode     string
	MaxTurns           int
	RequiredMcpServers []string
	SystemPrompt       string
	Source             string // "built-in" | "project" | "user"
	FilePath           string
}

func (d Definition) GetAgentType() string { return d.AgentType }

func envTruthy(key string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func isNonSdkEntrypoint() bool {
	switch strings.TrimSpace(strings.ToLower(os.Getenv("CLAUDE_CODE_ENTRYPOINT"))) {
	case "sdk-ts", "sdk-py", "sdk-cli":
		return false
	default:
		return true
	}
}

func areExplorePlanAgentsEnabled() bool {
	// TS: gated by GrowthBook experiment BUILTIN_EXPLORE_PLAN_AGENTS.
	// Go port: opt-in via env for deterministic testing.
	return envTruthy("CLAUDE_CODE_BUILTIN_EXPLORE_PLAN_AGENTS") || envTruthy("BUILTIN_EXPLORE_PLAN_AGENTS")
}

func isVerificationAgentEnabled() bool {
	// TS: feature('VERIFICATION_AGENT') + GB gate. Go port: opt-in via env.
	return envTruthy("CLAUDE_CODE_ENABLE_VERIFICATION_AGENT") || envTruthy("VERIFICATION_AGENT")
}

func builtInAgents() []Definition {
	// TS: allow disabling all built-in agents for SDK usage in noninteractive mode.
	if envTruthy("CLAUDE_AGENT_SDK_DISABLE_BUILTIN_AGENTS") && !isNonSdkEntrypoint() {
		return nil
	}

	// Prompts are intentionally kept as static strings in Go port for now.
	// (TS uses getSystemPrompt() functions with feature gates.)
	generalPurposePrompt := `You are an agent for Claude Code, Anthropic's official CLI for Claude. Given the user's message, you should use the tools available to complete the task. Complete the task fully—don't gold-plate, but don't leave it half-done.

When you complete the task, respond with a concise report covering what was done and any key findings — the caller will relay this to the user, so it only needs the essentials.

Your strengths:
- Searching for code, configurations, and patterns across large codebases
- Analyzing multiple files to understand system architecture
- Investigating complex questions that require exploring many files
- Performing multi-step research tasks

Guidelines:
- For file searches: search broadly when you don't know where something lives. Use Read when you know the specific file path.
- For analysis: Start broad and narrow down. Use multiple search strategies if the first doesn't yield results.
- Be thorough: Check multiple locations, consider different naming conventions, look for related files.
- NEVER create files unless they're absolutely necessary for achieving your goal. ALWAYS prefer editing an existing file to creating a new one.
- NEVER proactively create documentation files (*.md) or README files. Only create documentation files if explicitly requested.`

	explorePrompt := `You are a file search specialist for Claude Code. You excel at thoroughly navigating and exploring codebases.

=== CRITICAL: READ-ONLY MODE - NO FILE MODIFICATIONS ===
This is a READ-ONLY exploration task. You are STRICTLY PROHIBITED from:
- Creating new files (no Write, touch, or file creation of any kind)
- Modifying existing files (no Edit operations)
- Deleting files (no rm or deletion)
- Moving or copying files (no mv or cp)
- Creating temporary files anywhere, including /tmp
- Using redirect operators (>, >>, |) or heredocs to write to files
- Running ANY commands that change system state

Your role is EXCLUSIVELY to search and analyze existing code.

Guidelines:
- Use Glob for broad file pattern matching
- Use Grep for searching file contents with regex
- Use Read when you know the specific file path you need to read
- Use Bash ONLY for read-only operations (ls, git status, git log, git diff, find, grep, cat, head, tail)
- NEVER use Bash for: mkdir, touch, rm, cp, mv, git add, git commit, npm install, pip install, or any file creation/modification
- Wherever possible, spawn multiple parallel tool calls for grepping and reading files

Complete the user's search request efficiently and report your findings clearly.`

	planPrompt := `You are a software architect and planning specialist for Claude Code. Your role is to explore the codebase and design implementation plans.

=== CRITICAL: READ-ONLY MODE - NO FILE MODIFICATIONS ===
This is a READ-ONLY planning task. You are STRICTLY PROHIBITED from creating/modifying/deleting files or running commands that change system state.

Your Process:
1. Understand requirements and constraints.
2. Explore thoroughly using Glob/Grep/Read (Bash only for read-only ops).
3. Design solution and trade-offs.
4. Provide step-by-step implementation plan.

Required output:
End your response with:

### Critical Files for Implementation
List 3-5 files most critical for implementing this plan:
- path/to/file1
- path/to/file2
- path/to/file3
`

	claudeCodeGuidePrompt := `You are the Claude guide agent. Your job is to help users understand and use:
1) Claude Code (the CLI tool)
2) Claude Agent SDK (TypeScript/Python)
3) Claude API (Messages API, streaming, tools, MCP)

Approach:
- Prefer official docs. Use WebFetch to fetch the docs map first, then fetch the specific pages.
- Use WebSearch only if docs do not cover the topic.
- When relevant, reference local project files (CLAUDE.md, .claude/ directory) using Read/Glob/Grep.
`

	verificationPrompt := `You are a verification specialist. Your job is not to confirm the implementation works — it's to try to break it.

=== CRITICAL: DO NOT MODIFY THE PROJECT ===
You are STRICTLY PROHIBITED from creating/modifying/deleting files in the project directory, installing dependencies, or running git write operations.

You MUST provide evidence-based checks (commands run + observed output), and end with:
VERDICT: PASS
or VERDICT: FAIL
or VERDICT: PARTIAL
`

	statuslinePrompt := `You are a status line setup agent for Claude Code. Your job is to create or update the statusLine command in the user's Claude Code settings (~/.claude/settings.json).

You may read shell config (~/.zshrc, ~/.bashrc, ~/.bash_profile, ~/.profile) and convert PS1 into a statusLine command.
If ~/.claude/settings.json is a symlink, update the target file instead.
Preserve existing settings when updating.
`

	agents := []Definition{
		// Legacy Go port types (kept for compatibility with earlier runs/deltas).
		{AgentType: "search", WhenToUse: "Handle search tasks autonomously.", Source: "built-in"},
		{AgentType: "general_purpose_task", WhenToUse: "Perform a general-purpose coding task using a sub-agent.", Source: "built-in"},

		// TS built-ins (1:1 list, gated similarly).
		{
			AgentType:    "general-purpose",
			WhenToUse:    "General-purpose agent for researching complex questions, searching for code, and executing multi-step tasks.",
			Tools:        nil, // '*' semantics: all tools (subject to runtime filtering)
			Model:        "inherit",
			SystemPrompt: generalPurposePrompt,
			Source:       "built-in",
		},
		{
			AgentType:    "statusline-setup",
			WhenToUse:    "Use this agent to configure the user's Claude Code status line setting.",
			Tools:        []string{"Read", "Edit"},
			Model:        "sonnet",
			SystemPrompt: statuslinePrompt,
			Source:       "built-in",
		},
	}

	if areExplorePlanAgentsEnabled() {
		agents = append(agents,
			Definition{
				AgentType:       "Explore",
				WhenToUse:       "Fast agent specialized for exploring codebases (file patterns, keyword search, architecture questions).",
				DisallowedTools: []string{"Agent", "ExitPlanMode", "Edit", "Write"},
				Model:           "haiku",
				SystemPrompt:    explorePrompt,
				Source:          "built-in",
			},
			Definition{
				AgentType:       "Plan",
				WhenToUse:       "Software architect agent for designing implementation plans (step-by-step plan, critical files, trade-offs).",
				DisallowedTools: []string{"Agent", "ExitPlanMode", "Edit", "Write"},
				Model:           "inherit",
				SystemPrompt:    planPrompt,
				Source:          "built-in",
			},
		)
	}

	if isNonSdkEntrypoint() {
		agents = append(agents, Definition{
			AgentType:       "claude-code-guide",
			WhenToUse:       "Use this agent for questions about Claude Code, Claude Agent SDK, and Claude API; it should fetch official docs and answer with URLs.",
			Tools:           []string{"Read", "Glob", "Grep", "WebFetch", "WebSearch"},
			Model:           "haiku",
			PermissionMode:  "dontAsk",
			SystemPrompt:    claudeCodeGuidePrompt,
			Source:          "built-in",
		})
	}

	if isVerificationAgentEnabled() {
		agents = append(agents, Definition{
			AgentType:       "verification",
			WhenToUse:       "Use this agent to verify implementation correctness with command evidence and a PASS/FAIL/PARTIAL verdict.",
			DisallowedTools: []string{"Agent", "ExitPlanMode", "Edit", "Write"},
			Model:           "inherit",
			SystemPrompt:    verificationPrompt,
			Source:          "built-in",
		})
	}

	return agents
}

func parseFrontmatterBlock(markdown string) (map[string]any, bool) {
	// Very small YAML subset: key: value, key: (then list items), list items "- x".
	sc := bufio.NewScanner(strings.NewReader(markdown))
	if !sc.Scan() {
		return nil, false
	}
	if strings.TrimSpace(sc.Text()) != "---" {
		return nil, false
	}
	out := map[string]any{}
	var currentListKey string
	for sc.Scan() {
		line := sc.Text()
		trim := strings.TrimSpace(line)
		if trim == "---" {
			return out, true
		}
		if strings.HasPrefix(trim, "- ") && currentListKey != "" {
			item := strings.TrimSpace(strings.TrimPrefix(trim, "- "))
			if item == "" {
				continue
			}
			cur, _ := out[currentListKey].([]string)
			out[currentListKey] = append(cur, strings.Trim(item, `"'`))
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
			// Might be a list.
			out[key] = []string{}
			currentListKey = key
			continue
		}
		out[key] = strings.Trim(val, `"'`)
	}
	return nil, false
}

func parseStringList(v any) []string {
	switch t := v.(type) {
	case []string:
		out := make([]string, 0, len(t))
		for _, s := range t {
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		// Comma-separated convenience.
		parts := strings.Split(t, ",")
		out := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				out = append(out, p)
			}
		}
		return out
	default:
		return nil
	}
}

func parseAgentToolsFromFrontmatter(v any) []string {
	// TS semantics:
	// - missing => nil (all tools)
	// - empty => [] (no tools)
	// - "*" => nil (all tools)
	if v == nil {
		return nil
	}
	list := parseStringList(v)
	if list == nil {
		return []string{}
	}
	if len(list) == 0 {
		return []string{}
	}
	expanded := make([]string, 0, len(list))
	for _, item := range list {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		// Support comma-separated items within a single string entry.
		for _, part := range strings.Split(item, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			if part == "*" {
				return nil
			}
			expanded = append(expanded, part)
		}
	}
	return expanded
}

func parseModel(v any) string {
	s, _ := v.(string)
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.EqualFold(s, "inherit") {
		return "inherit"
	}
	return s
}

func parsePermissionMode(v any) string {
	s, _ := v.(string)
	s = strings.TrimSpace(strings.ToLower(s))
	switch s {
	case "default", "accepted-edits", "bypass", "plan", "ask", "dontask", "bubble":
		return s
	default:
		return ""
	}
}

func parsePositiveInt(v any) int {
	switch t := v.(type) {
	case string:
		t = strings.TrimSpace(t)
		if t == "" {
			return 0
		}
		n := 0
		for _, ch := range t {
			if ch < '0' || ch > '9' {
				return 0
			}
			n = n*10 + int(ch-'0')
		}
		if n > 0 {
			return n
		}
	case int:
		if t > 0 {
			return t
		}
	case int64:
		if t > 0 {
			return int(t)
		}
	case float64:
		if t > 0 && float64(int(t)) == t {
			return int(t)
		}
	}
	return 0
}

func parseAgentMarkdown(filePath string, markdown string, source string) *Definition {
	fm, ok := parseFrontmatterBlock(markdown)
	if !ok {
		return nil
	}
	name, _ := fm["name"].(string)
	desc, _ := fm["description"].(string)
	name = strings.TrimSpace(name)
	desc = strings.TrimSpace(desc)
	if name == "" || desc == "" {
		return nil
	}
	tools := parseAgentToolsFromFrontmatter(fm["tools"])
	disallowedTools := parseAgentToolsFromFrontmatter(fm["disallowedTools"])
	model := parseModel(fm["model"])
	permissionMode := parsePermissionMode(fm["permissionMode"])
	maxTurns := parsePositiveInt(fm["maxTurns"])
	req := parseStringList(fm["requiredMcpServers"])
	def := &Definition{
		AgentType:          name,
		WhenToUse:          strings.ReplaceAll(desc, `\n`, "\n"),
		RequiredMcpServers: req,
		Source:             source,
		FilePath:           filePath,
	}
	if tools != nil {
		def.Tools = tools
	}
	if disallowedTools != nil {
		def.DisallowedTools = disallowedTools
	}
	if model != "" {
		def.Model = model
	}
	if permissionMode != "" {
		def.PermissionMode = permissionMode
	}
	if maxTurns > 0 {
		def.MaxTurns = maxTurns
	}

	// Everything after the frontmatter block is the agent's system prompt.
	// This mirrors TS parseAgentFromMarkdown: `const systemPrompt = content.trim()`.
	if idx := strings.Index(markdown, "\n---"); idx >= 0 {
		// Find the second '---' line end.
		rest := markdown[idx+4:]
		if idx2 := strings.Index(rest, "\n---"); idx2 >= 0 {
			body := strings.TrimSpace(rest[idx2+4:])
			def.SystemPrompt = body
		}
	}
	return def
}

func LoadAll(cwd string) []Definition {
	defs := make([]Definition, 0, 32)
	defs = append(defs, builtInAgents()...)

	candidates := []struct {
		dir    string
		source string
	}{
		{dir: filepath.Join(cwd, ".claude", "agents"), source: "project"},
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates, struct {
			dir    string
			source string
		}{dir: filepath.Join(home, ".claude", "agents"), source: "user"})
	}
	for _, c := range candidates {
		entries, err := os.ReadDir(c.dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			low := strings.ToLower(e.Name())
			if !strings.HasSuffix(low, ".md") {
				continue
			}
			p := filepath.Join(c.dir, e.Name())
			b, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			if d := parseAgentMarkdown(p, string(b), c.source); d != nil {
				defs = append(defs, *d)
			}
		}
	}

	// Last-wins by agentType (TS groups by source priority).
	byType := map[string]Definition{}
	for _, d := range defs {
		byType[d.AgentType] = d
	}
	out := make([]Definition, 0, len(byType))
	for _, d := range byType {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].AgentType < out[j].AgentType })
	return out
}

func HasRequiredMcpServers(agent Definition, availableServers []string) bool {
	if len(agent.RequiredMcpServers) == 0 {
		return true
	}
	for _, pattern := range agent.RequiredMcpServers {
		pattern = strings.TrimSpace(strings.ToLower(pattern))
		if pattern == "" {
			continue
		}
		found := false
		for _, s := range availableServers {
			if strings.Contains(strings.ToLower(s), pattern) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func FilterByMcpRequirements(agents []Definition, availableServers []string) []Definition {
	out := make([]Definition, 0, len(agents))
	for _, a := range agents {
		if HasRequiredMcpServers(a, availableServers) {
			out = append(out, a)
		}
	}
	return out
}

func FormatLines(defs []Definition) []string {
	out := make([]string, 0, len(defs))
	for _, d := range defs {
		out = append(out, "- "+d.AgentType+" — "+strings.TrimSpace(d.WhenToUse))
	}
	return out
}
