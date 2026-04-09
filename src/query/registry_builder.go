package query

import (
	"context"
	"os"
	"strings"

	"claude-code-running-go/src/tool"
	"claude-code-running-go/src/tools"
	"claude-code-running-go/src/tools/agent"
	"claude-code-running-go/src/tools/askuserquestion"
	"claude-code-running-go/src/tools/bash"
	"claude-code-running-go/src/tools/discoverskills"
	"claude-code-running-go/src/tools/fileedit"
	"claude-code-running-go/src/tools/fileread"
	"claude-code-running-go/src/tools/filewrite"
	"claude-code-running-go/src/tools/glob"
	"claude-code-running-go/src/tools/grep"
	"claude-code-running-go/src/tools/skill"
	"claude-code-running-go/src/tools/todowrite"
	toolsearchtool "claude-code-running-go/src/tools/toolsearch"
	"claude-code-running-go/src/tools/webfetch"
	"claude-code-running-go/src/tools/websearch"
	"claude-code-running-go/src/utils/agents"
	toolsearchutil "claude-code-running-go/src/utils/toolsearch"
)

// RegistryBuilder allows injecting tool pool construction for testing/extension.
// It is the primary seam for changing which tools are available without editing QueryEngine.
type RegistryBuilder interface {
	Build(ctx context.Context, cfg QueryEngineConfig, systemPrompt []string) (*tools.Registry, error)
}

// ModelResolver is an injection point to customize how the model is chosen per turn.
// Default: use override when set, otherwise use client.Model().
type ModelResolver func(client modelResolverClient, override string) string

type modelResolverClient interface {
	Model() string
}

func defaultModelResolver(client modelResolverClient, override string) string {
	if override != "" {
		return override
	}
	return client.Model()
}

func resolveModel(cfg QueryEngineConfig) string {
	override := strings.TrimSpace(cfg.ModelOverride)
	if cfg.ModelResolver != nil {
		return cfg.ModelResolver(cfg.Client, override)
	}
	return defaultModelResolver(cfg.Client, override)
}

// DefaultRegistryBuilder preserves the current hard-coded tool set as the default behavior.
// Tests can replace this with a minimal builder to avoid starting MCP, running Bash, etc.
type DefaultRegistryBuilder struct{}

func (b DefaultRegistryBuilder) Build(ctx context.Context, cfg QueryEngineConfig, systemPrompt []string) (*tools.Registry, error) {
	_ = ctx
	reg := tools.NewRegistry([]tool.Tool{
		fileread.New(),
		fileedit.New(),
		filewrite.New(),
		bash.New(),
		glob.New(),
		grep.New(),
		webfetch.New(),
		websearch.New(),
		todowrite.New(),
		askuserquestion.New(),
		discoverskills.New(cfg.SkillDirs),
		agent.NewWithConfig(agent.Config{
			BaseSystemPrompt: systemPrompt,
			ParentModel:      resolveModel(cfg),
			ParentMode:       cfg.PermissionMode,
			Resolve: func(agentType string) (agent.AgentSpec, bool) {
				cwd, _ := os.Getwd()
				defs := agents.LoadAll(cwd)
				for _, d := range defs {
					if d.AgentType != agentType {
						continue
					}
					return agent.AgentSpec{
						AgentType:          d.AgentType,
						SystemPrompt:       d.SystemPrompt,
						Tools:              d.Tools,
						DisallowedTools:    d.DisallowedTools,
						Model:              d.Model,
						PermissionMode:     d.PermissionMode,
						MaxTurns:           d.MaxTurns,
						RequiredMcpServers: d.RequiredMcpServers,
					}, true
				}
				return agent.AgentSpec{}, false
			},
			Run: func(ctx context.Context, opts agent.RunOptions) (string, error) {
				childCfg := QueryEngineConfig{
					Client:          cfg.Client,
					SystemPrompt:    opts.SystemPrompt,
					PermissionMode:  opts.PermissionMode,
					AllowedTools:    opts.AllowedTools,
					DisallowedTools: opts.DisallowedTools,
					MCPConfigPath:   cfg.MCPConfigPath,
					PluginDirs:      cfg.PluginDirs,
					SkillDirs:       cfg.SkillDirs,
					Debug:           cfg.Debug,
					MessagesDumpDir: cfg.MessagesDumpDir,
					ModelOverride:   opts.Model,
					MaxSteps:        opts.MaxTurns,
					RegistryBuilder: cfg.RegistryBuilder,
					ModelResolver:   cfg.ModelResolver,
					DiscoveredToolsPath:        cfg.DiscoveredToolsPath,
					MaxDiscoveredDeferredTools: cfg.MaxDiscoveredDeferredTools,
					TranscriptPath:             cfg.TranscriptPath,
					ResumeTranscript:           cfg.ResumeTranscript,
				}
				out, err := NewQueryEngine(childCfg).RunOnce(ctx, opts.UserPrompt)
				if err != nil {
					return "", err
				}
				return out.Text, nil
			},
			DepthLimit: 1,
		}),
		skill.New(cfg.SkillDirs),
	})

	// ToolSearch should discover only deferred tools (currently MCP tools), and it must reflect
	// the current registry at call time, so we provide a closure.
	reg.Add(toolsearchtool.New(func() []tool.Tool {
		out := make([]tool.Tool, 0, 32)
		for _, tt := range reg.List() {
			if toolsearchutil.IsDeferredTool(tt) {
				out = append(out, tt)
			}
		}
		return out
	}))

	return reg, nil
}
