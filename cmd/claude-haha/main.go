package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"claude-code-running-go/src/entrypoints"
)

func main() {
	var (
		printMode          bool
		dumpSystemPrompt   bool
		debug              bool
		messagesDump       string
		model              string
		baseURL            string
		apiKey             string
		authToken          string
		permissionMode     string
		allowedTools       string
		disallowedTools    string
		mcpConfigPath      string
		pluginDirs         string
		pluginDirFlags     stringList
		skillDirFlags      stringList
		systemPrompt       string
		appendSystemPrompt string
	)

	flag.BoolVar(&printMode, "print", false, "Print response and exit (non-interactive)")
	flag.BoolVar(&printMode, "p", false, "Alias for --print")
	flag.BoolVar(&dumpSystemPrompt, "dump-system-prompt", false, "Dump effective system prompt and exit")
	flag.BoolVar(&debug, "debug", false, "Enable debug logging (system prompt, tool schemas, messages, permission decisions)")
	flag.StringVar(&messagesDump, "messages-dump", "", "Dump per-turn request/response/messages/tool_results to a directory. Use '1' to pick a default directory.")
	flag.StringVar(&model, "model", "", "Model id or alias (e.g. sonnet/haiku/opus/best); defaults to env ANTHROPIC_MODEL")
	flag.StringVar(&baseURL, "base-url", "", "Anthropic-compatible base URL (defaults to env ANTHROPIC_BASE_URL)")
	flag.StringVar(&apiKey, "api-key", "", "API key (defaults to env ANTHROPIC_API_KEY)")
	flag.StringVar(&authToken, "auth-token", "", "Auth token (defaults to env ANTHROPIC_AUTH_TOKEN)")
	flag.StringVar(&permissionMode, "permission-mode", "default", `Permission mode: "default" | "ask" | "bypass"`)
	flag.StringVar(&allowedTools, "allowed-tools", "", `Comma-separated tool allow list (e.g. "Read,Glob")`)
	flag.StringVar(&disallowedTools, "disallowed-tools", "", `Comma-separated tool deny list (e.g. "Bash,Edit")`)
	flag.StringVar(&mcpConfigPath, "mcp-config", "", "Path to MCP config file (e.g. .mcp.json). If empty, uses default search paths.")
	flag.StringVar(&pluginDirs, "plugin-dirs", "", `Colon-separated plugin directories (scan for .mcp.json in plugins). Example: "/path/a:/path/b"`)
	flag.Var(&pluginDirFlags, "plugin-dir", "Plugin directory (repeatable). Scanned for plugins containing .mcp.json / plugin.json mcpServers.")
	flag.Var(&skillDirFlags, "skill-dir", "Skills directory (repeatable). Used by Skill tool to locate installed skills.")
	flag.StringVar(&systemPrompt, "system-prompt", "", "Override system prompt for the session")
	flag.StringVar(&appendSystemPrompt, "append-system-prompt", "", "Append content after default system prompt")
	flag.Parse()

	prompt := strings.TrimSpace(strings.Join(flag.Args(), " "))
	if printMode && prompt == "" && !dumpSystemPrompt {
		// Pipe-friendly: allow stdin as prompt.
		if stat, err := os.Stdin.Stat(); err == nil && (stat.Mode()&os.ModeCharDevice) == 0 {
			b, _ := os.ReadFile("/dev/stdin")
			prompt = strings.TrimSpace(string(b))
		}
	}
	if prompt == "" && !dumpSystemPrompt {
		fmt.Fprintln(os.Stderr, "No prompt provided. Example: ./cmd/claude-haha --print \"hello\"")
		os.Exit(2)
	}

	ctx := context.Background()
	out, err := entrypoints.RunCLI(ctx, entrypoints.CLIOptions{
		PrintMode:          printMode,
		DumpSystemPrompt:   dumpSystemPrompt,
		Debug:              debug,
		MessagesDumpDir:    normalizeMessagesDump(messagesDump),
		Prompt:             prompt,
		Model:              model,
		BaseURL:            baseURL,
		APIKey:             apiKey,
		AuthToken:          authToken,
		PermissionMode:     permissionMode,
		AllowedTools:       splitCSV(allowedTools),
		DisallowedTools:    splitCSV(disallowedTools),
		MCPConfigPath:      strings.TrimSpace(mcpConfigPath),
		PluginDirs:         append(splitPathList(pluginDirs), pluginDirFlags...),
		SkillDirs:          skillDirFlags,
		SystemPrompt:       systemPrompt,
		AppendSystemPrompt: appendSystemPrompt,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	fmt.Println(out)
}

func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func splitPathList(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(s, string(os.PathListSeparator))
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// stringList implements flag.Value.
type stringList []string

func (s *stringList) String() string { return strings.Join(*s, string(os.PathListSeparator)) }

func (s *stringList) Set(v string) error {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	*s = append(*s, v)
	return nil
}

func normalizeMessagesDump(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	l := strings.ToLower(v)
	if l == "1" || l == "true" || l == "yes" || l == "on" {
		// Timestamped directory under repo-local debug area.
		return filepath.Join(".claude-go", "messages-dump", time.Now().Format("20060102-150405"))
	}
	return v
}
