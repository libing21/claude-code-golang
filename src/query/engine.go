package query

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"
	"time"

	"claude-code-running-go/src/constants/prompts"
	"claude-code-running-go/src/memdir"
	"claude-code-running-go/src/services/api"
	"claude-code-running-go/src/services/mcp"
	toolruntime "claude-code-running-go/src/services/tools"
	"claude-code-running-go/src/tool"
	"claude-code-running-go/src/tools"
	mcptool "claude-code-running-go/src/tools/mcp"
	"claude-code-running-go/src/utils/agents"
	"claude-code-running-go/src/utils/attachments"
	"claude-code-running-go/src/utils/plugins"
	toolsearchutil "claude-code-running-go/src/utils/toolsearch"
)

// QueryEngine is a Go analog of TS QueryEngine.ts.
type QueryEngine struct {
	cfg QueryEngineConfig

	// Turn state (mirrors TS QueryEngine owning mutable messages/context).
	reg           *tools.Registry
	permCtx       tool.PermissionContext
	attachmentReg *attachments.Registry
	msgs          []api.Message
	lastEmittedDate string

	exposedTools []tool.Tool
	exposure     toolExposure
	orch         *toolruntime.ToolOrchestration
	transcript   TranscriptStore
	toolBudget   *ToolBudgetState
	budgetTracker *BudgetTracker
	lastFileWriteBatchFailed bool
	lastFileWriteFailSnippets []string

	// systemPrompt is mutable because MCP instructions section may be replaced.
	systemPrompt []string
}

type QueryEngineConfig struct {
	Client          *api.Client
	SystemPrompt    []string
	PermissionMode  string
	AllowedTools    []string
	DisallowedTools []string
	MCPConfigPath   string
	PluginDirs      []string
	SkillDirs       []string
	Debug           bool
	MessagesDumpDir string
	ModelOverride   string
	MaxSteps        int

	// Injection points for extensibility & testing.
	RegistryBuilder RegistryBuilder
	ModelResolver   ModelResolver

	// DiscoveredToolsPath persists the discovered deferred tool set so defer_loading
	// remains stable across restarts (and later: compact/resume). If empty, defaults
	// to "<cwd>/.claude-go/discovered-tools.json".
	DiscoveredToolsPath string
	// MaxDiscoveredDeferredTools caps the number of discovered deferred tools kept.
	MaxDiscoveredDeferredTools int

	TranscriptPath   string
	ResumeTranscript bool

	StopHookRunner           StopHookRunner
	TokenBudgetTokens        int
	AutoCompactTokenThreshold int
	AutoCompactKeepMessages   int
}

func NewQueryEngine(cfg QueryEngineConfig) *QueryEngine {
	if cfg.MaxSteps <= 0 {
		cfg.MaxSteps = 8
	}
	if cfg.MaxDiscoveredDeferredTools <= 0 {
		cfg.MaxDiscoveredDeferredTools = 256
	}
	if !cfg.ResumeTranscript && envTruthy("CLAUDE_GO_RESUME_TRANSCRIPT") {
		cfg.ResumeTranscript = true
	}
	if cfg.StopHookRunner == nil {
		cfg.StopHookRunner = NoopStopHookRunner{}
	}
	if cfg.TokenBudgetTokens <= 0 {
		if envTruthy("CLAUDE_GO_ENABLE_TOKEN_BUDGET") {
			cfg.TokenBudgetTokens = 120000
		}
	}
	if cfg.AutoCompactTokenThreshold <= 0 {
		if envTruthy("CLAUDE_GO_ENABLE_AUTO_COMPACT") {
			cfg.AutoCompactTokenThreshold = 160000
		}
	}
	if cfg.AutoCompactKeepMessages <= 0 {
		cfg.AutoCompactKeepMessages = 12
	}
	return &QueryEngine{
		cfg:           cfg,
		systemPrompt:  append([]string{}, cfg.SystemPrompt...),
		budgetTracker: NewBudgetTracker(),
	}
}

func (e *QueryEngine) modelUsedForTurn() string {
	return resolveModel(e.cfg)
}

func (e *QueryEngine) RunOnce(ctx context.Context, userPrompt string) (Output, error) {
	if err := e.initRegistry(ctx); err != nil {
		return Output{}, err
	}
	e.initPermissionContext()
	if err := e.initTranscript(); err != nil {
		return Output{}, err
	}
	e.initAttachments()
	e.loadMCP(ctx)
	e.initOrchestration()
	e.announceAgentListingDelta()
	e.announceDeferredToolsDelta()
	e.injectRelevantMemories(ctx, userPrompt)
	e.appendMessage(api.Message{Role: "user", Content: userPrompt})
	return e.runSteps(ctx)
}

func (e *QueryEngine) initTranscript() error {
	e.transcript = NewFileTranscriptStore(strings.TrimSpace(e.cfg.TranscriptPath))
	if !e.cfg.ResumeTranscript {
		e.toolBudget = NewToolBudgetState()
		return nil
	}
	msgs, err := e.transcript.Load()
	if err != nil {
		return err
	}
	e.msgs = append(e.msgs, msgs...)
	e.toolBudget = ReconstructToolBudgetState(e.msgs)
	return nil
}

func (e *QueryEngine) initRegistry(ctx context.Context) error {
	builder := e.cfg.RegistryBuilder
	if builder == nil {
		builder = DefaultRegistryBuilder{}
	}
	reg, err := builder.Build(ctx, e.cfg, e.systemPrompt)
	if err != nil {
		return err
	}
	e.reg = reg
	return nil
}

func (e *QueryEngine) initPermissionContext() {
	permMode := tool.PermissionModeDefault
	switch strings.ToLower(strings.TrimSpace(e.cfg.PermissionMode)) {
	case "bypass":
		permMode = tool.PermissionModeBypass
	case "ask":
		permMode = tool.PermissionModeAsk
	case "default", "":
		permMode = tool.PermissionModeDefault
	}
	e.permCtx = tool.PermissionContext{Mode: permMode}
	e.permCtx.AllowRules = tool.BuildRulesFromStrings("cliArg", tool.PermissionBehaviorAllow, e.cfg.AllowedTools)
	e.permCtx.DenyRules = tool.BuildRulesFromStrings("cliArg", tool.PermissionBehaviorDeny, e.cfg.DisallowedTools)
	e.permCtx.AskRules = tool.BuildRulesFromStrings("cliArg", tool.PermissionBehaviorAsk, nil)
}

func (e *QueryEngine) initAttachments() {
	if e.msgs == nil {
		e.msgs = []api.Message{}
	}
	e.attachmentReg = attachments.NewRegistry()
	e.attachmentReg.Register(attachments.DateChangeProvider{})
	e.attachmentReg.Register(attachments.CriticalSystemReminderProvider{})
	e.attachmentReg.Register(attachments.OutputStyleProvider{})
	e.attachmentReg.Register(attachments.PlanModeReentryProvider{})
	e.attachmentReg.Register(attachments.PlanModeProvider{})
	e.attachmentReg.Register(attachments.PlanModeExitProvider{})
	e.attachmentReg.Register(attachments.AutoModeProvider{})
	e.attachmentReg.Register(attachments.AutoModeExitProvider{})
	e.attachmentReg.Register(attachments.DeferredToolsDeltaProvider{})
	e.attachmentReg.Register(attachments.AgentListingDeltaProvider{})
	e.attachmentReg.Register(attachments.RelevantMemoriesProvider{})
	e.attachmentReg.Register(attachments.McpInstructionsDeltaProvider{})
	e.lastEmittedDate = time.Now().Format("2006-01-02")

	e.appendMessages(e.attachmentReg.BuildMessages(attachments.Context{
		CriticalSystemReminder: strings.TrimSpace(os.Getenv("CLAUDE_GO_CRITICAL_SYSTEM_REMINDER")),
		OutputStyleName:        strings.TrimSpace(os.Getenv("CLAUDE_GO_OUTPUT_STYLE_NAME")),
		PlanMode:               inferPlanModeAttachmentFromMessages(e.cfg.PermissionMode, e.msgs),
		PlanModeReentry:        inferPlanModeReentryAttachment(),
		PlanModeExit:           inferPlanModeExitAttachment(),
		AutoMode:               inferAutoModeAttachmentFromMessages(e.cfg.PermissionMode, e.msgs),
		AutoModeExit:           envTruthy("CLAUDE_GO_AUTO_MODE_EXIT"),
	}))
}

func (e *QueryEngine) initOrchestration() {
	e.exposedTools = filterToolsForSchema(e.reg.List(), e.cfg.AllowedTools, e.cfg.DisallowedTools)
	e.exposure = buildToolExposure(e.cfg.AllowedTools, e.cfg.DisallowedTools)
	e.orch = toolruntime.NewToolOrchestration(e.exposedTools)

	// Seed discovered deferred-tools set from disk (best-effort).
	storePath := strings.TrimSpace(e.cfg.DiscoveredToolsPath)
	if storePath == "" {
		cwd, _ := os.Getwd()
		storePath = cwd + string(os.PathSeparator) + ".claude-go" + string(os.PathSeparator) + "discovered-tools.json"
	}
	e.orch.SetDiscoveredToolsStore(toolruntime.NewFileDiscoveredToolsStore(storePath), e.cfg.MaxDiscoveredDeferredTools)
	_ = e.orch.LoadDiscovered()
	e.orch.ReplayDiscoveredFromMessages(e.msgs)
}

func (e *QueryEngine) injectRelevantMemories(ctx context.Context, userPrompt string) {
	if !memdir.IsAutoMemoryEnabled() {
		return
	}
	cwd, _ := os.Getwd()
	memoryDir := memdir.GetAutoMemPath(cwd)
	rels, _ := memdir.FindRelevantMemories(ctx, e.cfg.Client, userPrompt, memoryDir, nil, nil)
	extracted, _ := memdir.ExtractRelevantMemories(ctx, rels)
	e.appendMessages(e.attachmentReg.BuildMessages(attachments.Context{
		RelevantMemories: extracted,
	}))
}

func (e *QueryEngine) runSteps(ctx context.Context) (Output, error) {
	finalTexts := []string{}
	for step := 0; step < e.cfg.MaxSteps; step++ {
		done, err := e.step(ctx, step, &finalTexts)
		if err != nil {
			return Output{}, err
		}
		if done {
			break
		}
	}
	if len(finalTexts) == 0 {
		return Output{}, fmt.Errorf("empty response")
	}
	return Output{Text: strings.Join(finalTexts, "\n")}, nil
}

func (e *QueryEngine) step(ctx context.Context, step int, finalTexts *[]string) (bool, error) {
	if compacted, info := maybeCompactMessages(e.msgs, e.cfg.AutoCompactTokenThreshold, e.cfg.AutoCompactKeepMessages); info != nil && info.Triggered {
		e.msgs = compacted
		if e.cfg.Debug {
			fmt.Fprintf(os.Stderr, "[debug] compact triggered step=%d omitted=%d est_tokens=%d\n", step, info.OmittedCount, info.EstimatedTokens)
		}
	}
	currentDate := time.Now().Format("2006-01-02")
	if currentDate != e.lastEmittedDate {
		e.appendMessages(e.attachmentReg.BuildMessages(attachments.Context{
			DateChanged: true,
			NewDate:     currentDate,
		}))
		e.lastEmittedDate = currentDate
	}
	if e.cfg.Debug {
		fmt.Fprintf(os.Stderr, "[debug] step=%d messages=%d\n", step, len(e.msgs))
	}

	modelUsed := e.modelUsedForTurn()
	e.orch.DeferLoading = e.exposure.IsExposed("ToolSearch") &&
		toolsearchutil.IsEnabled(modelUsed, e.exposedTools)
	toolSchemas := e.orch.BuildToolSchemas()

	var streamEvents []api.RawMessageStreamEvent
	resp, err := e.cfg.Client.CreateMessageStreaming(ctx, api.CreateMessageInput{
		SystemPrompt: e.systemPrompt,
		Messages:     e.msgs,
		Tools:        toolSchemas,
		Model:        e.cfg.ModelOverride,
	}, func(ev api.RawMessageStreamEvent) {
		if strings.TrimSpace(e.cfg.MessagesDumpDir) == "" {
			return
		}
		streamEvents = append(streamEvents, ev)
	})
	if err != nil {
		// Best-effort: some providers/proxies may not support SSE. Fall back to non-streaming.
		if e.cfg.Debug {
			fmt.Fprintf(os.Stderr, "[debug] streaming failed, fallback to non-streaming: %v\n", err)
		}
		resp, err = e.cfg.Client.CreateMessage(ctx, api.CreateMessageInput{
			SystemPrompt: e.systemPrompt,
			Messages:     e.msgs,
			Tools:        toolSchemas,
			Model:        e.cfg.ModelOverride,
		})
		if err != nil {
			return false, err
		}
	}
	if strings.TrimSpace(e.cfg.MessagesDumpDir) != "" {
		_ = dumpTurn(e.cfg.MessagesDumpDir, step, e.systemPrompt, toolSchemas, e.msgs, resp)
		dumpStreamEvents(e.cfg.MessagesDumpDir, step, streamEvents)
	}
	e.appendMessage(api.Message{Role: "assistant", Content: resp.Content})

	toolUses := []api.ContentBlock{}
	stepText := ""
	for _, b := range resp.Content {
		switch b.Type {
		case "text":
			if strings.TrimSpace(b.Text) != "" {
				*finalTexts = append(*finalTexts, b.Text)
				stepText += "\n" + b.Text
			}
		case "tool_use":
			toolUses = append(toolUses, b)
		}
	}
	if len(toolUses) == 0 {
		// Hard guard: if the most recent file-write batch failed, do not allow
		// the assistant to claim success in the closing message.
		if e.lastFileWriteBatchFailed && shouldBlockSuccessTone(stepText) {
			msg := "注意：上一轮尝试写入/创建文件的工具调用全部失败，所以无法确认文件已生成。\n"
			if len(e.lastFileWriteFailSnippets) > 0 {
				msg += "失败原因（节选）：\n- " + strings.Join(e.lastFileWriteFailSnippets, "\n- ") + "\n"
			}
			msg += "请使用 `--permission-mode bypass` 或通过 `--allowed-tools Write,Bash` 放行后重试。"
			// Append a deterministic guard message so the final output can't end in a success tone.
			*finalTexts = append(*finalTexts, msg)
		}
		if e.cfg.StopHookRunner != nil {
			hookResult, err := e.cfg.StopHookRunner.AfterTurn(ctx, e.msgs, resp.Content)
			if err != nil {
				return false, err
			}
			e.appendMessages(hookResult.Messages)
			if hookResult.PreventContinuation {
				return true, nil
			}
		}
		if e.cfg.TokenBudgetTokens > 0 {
			decision := checkTokenBudget(e.budgetTracker, e.cfg.TokenBudgetTokens, approximateTokenCount(e.msgs))
			if decision.Action == "continue" {
				e.appendMessage(api.Message{Role: "user", Content: decision.NudgeMessage})
				return false, nil
			}
			if decision.CompletionEventText != "" && e.cfg.Debug {
				fmt.Fprintf(os.Stderr, "[debug] token budget stop: %s\n", decision.CompletionEventText)
			}
		}
		return true, nil
	}

	toolResultBlocks, toolUseCtx := e.executeTools(ctx, toolUses)
	if st := computeFileWriteBatchStatus(toolUses, toolResultBlocks); st.Attempted > 0 && st.Succeeded == 0 {
		e.lastFileWriteBatchFailed = true
		e.lastFileWriteFailSnippets = st.FailSnippets
		// Prevent the model from "hallucinating" success after a failed file write.
		e.appendMessage(api.Message{Role: "user", Content: "<system-reminder>Previous attempt to create/update a file via tools failed. Do NOT claim the file was created. Instead, report the failure and ask the user to rerun with --permission-mode bypass or allow the relevant tools (e.g. --allowed-tools Write,Bash).</system-reminder>"})
	} else if st.Attempted > 0 && st.Succeeded > 0 {
		e.lastFileWriteBatchFailed = false
		e.lastFileWriteFailSnippets = nil
	}
	e.orch.UpdateDiscoveredFromToolResults(toolResultBlocks)
	_ = e.orch.SaveDiscovered()
	if strings.TrimSpace(e.cfg.MessagesDumpDir) != "" {
		dumpToolProgress(e.cfg.MessagesDumpDir, step, toolUseCtx)
	}

	raw, _ := json.Marshal(toolResultBlocks)
	var contentBlocks []map[string]any
	_ = json.Unmarshal(raw, &contentBlocks)
	e.appendMessage(api.Message{Role: "user", Content: contentBlocks})
	return false, nil
}

func (e *QueryEngine) executeTools(ctx context.Context, toolUses []api.ContentBlock) ([]api.ToolResultBlock, *toolruntime.ToolUseContext) {
	toolUseCtx := toolruntime.NewToolUseContext(ctx)

	// TS: abortReason 'interrupt' means user provided new input while tools run.
	// In Go CLI we treat SIGINT (Ctrl+C) as an interrupt signal and only cancel cancelable tools.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt)
	go func() {
		select {
		case <-sigCh:
			toolUseCtx.Interrupt()
		case <-ctx.Done():
		}
	}()

	exec := toolruntime.NewStreamingToolExecutor(ctx, e.reg, e.permCtx, toolUseCtx, &toolruntime.Options{Debug: e.cfg.Debug})
	toolResultBlocks := exec.Execute(toolUses)
	signal.Stop(sigCh)
	return toolResultBlocks, toolUseCtx
}

func (e *QueryEngine) loadMCP(ctx context.Context) {
	servers := map[string]mcp.ServerConfig{}
	mcpInstructionClients := make([]prompts.MCPInstructionsClient, 0, 8)
	mcpInstructionsSection := ""

	{
		var cfg mcp.ConfigFile
		var err error
		if strings.TrimSpace(e.cfg.MCPConfigPath) != "" {
			cfg, err = mcp.LoadConfigFile(e.cfg.MCPConfigPath)
		} else {
			cfg, _, err = mcp.LoadFirstExistingConfig(mcp.FindDefaultConfigPaths())
		}
		if err == nil {
			for k, v := range cfg.McpServers {
				servers[k] = v
			}
		}
	}
	for k, v := range plugins.LoadPluginMcpServers(e.cfg.PluginDirs) {
		servers[k] = v
	}

	if len(servers) > 0 {
		for serverName, sc := range servers {
			mc, err := mcp.StartServer(serverName, sc)
			if err != nil {
				continue
			}
			_ = mcp.Initialize(ctx, mc)
			if inst := strings.TrimSpace(mc.Instructions()); inst != "" {
				mcpInstructionClients = append(mcpInstructionClients, prompts.MCPInstructionsClient{
					Name:         serverName,
					Instructions: inst,
				})
			}
			defs, err := mcp.ListTools(ctx, mc)
			if err != nil {
				_ = mc.Close()
				continue
			}
			for _, def := range defs {
				exposed := def.Name
				if _, ok := e.reg.Get(exposed); ok {
					exposed = serverName + "__" + def.Name
				}
				e.reg.Add(mcptool.New(serverName, mc, def, exposed))
			}
		}
	}

	mcpInstructionsSection = prompts.BuildMcpInstructionsSection(mcpInstructionClients)
	if mcp.IsMcpInstructionsDeltaEnabled() {
		e.systemPrompt = prompts.ReplaceSection(e.systemPrompt, prompts.SYSTEM_PROMPT_MCP_INSTRUCTIONS_SLOT, "")
	} else {
		e.systemPrompt = prompts.ReplaceSection(e.systemPrompt, prompts.SYSTEM_PROMPT_MCP_INSTRUCTIONS_SLOT, mcpInstructionsSection)
	}

	e.appendMessages(e.attachmentReg.BuildMessages(attachments.Context{
		McpInstructionsSection: mcpInstructionsSection,
		McpInstructionsDelta:   mcp.IsMcpInstructionsDeltaEnabled(),
	}))
}

func (e *QueryEngine) serversWithTools() []string {
	serverNames := map[string]struct{}{}
	for _, t := range e.reg.List() {
		if mt, ok := t.(interface {
			IsMCPTool() bool
			MCPServerName() string
		}); ok && mt.IsMCPTool() {
			serverNames[mt.MCPServerName()] = struct{}{}
		}
	}
	out := make([]string, 0, len(serverNames))
	for n := range serverNames {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

func (e *QueryEngine) announceAgentListingDelta() {
	if e.exposure.allowedSet == nil {
		// initOrchestration not run yet; be conservative.
		return
	}
	if !e.exposure.IsExposed("Agent") {
		return
	}
	serversWithTools := e.serversWithTools()
	cwd, _ := os.Getwd()
	allAgents := agents.LoadAll(cwd)
	filtered := agents.FilterByMcpRequirements(allAgents, serversWithTools)
	filtered = tool.FilterDeniedAgents(filtered, e.permCtx, "Agent")
	if allowed := tool.AllowedAgentTypesFromRules(e.permCtx, "Agent"); allowed != nil {
		allowSet := map[string]struct{}{}
		for _, t := range allowed {
			allowSet[t] = struct{}{}
		}
		tmp := make([]agents.Definition, 0, len(filtered))
		for _, a := range filtered {
			if _, ok := allowSet[a.AgentType]; ok {
				tmp = append(tmp, a)
			}
		}
		filtered = tmp
	}

	announced := scanAnnouncedAgentTypes(e.msgs)
	currentTypes := map[string]struct{}{}
	for _, a := range filtered {
		currentTypes[a.AgentType] = struct{}{}
	}
	added := make([]agents.Definition, 0, len(filtered))
	for _, a := range filtered {
		if _, ok := announced[a.AgentType]; !ok {
			added = append(added, a)
		}
	}
	removed := make([]string, 0, len(announced))
	for t := range announced {
		if _, ok := currentTypes[t]; !ok {
			removed = append(removed, t)
		}
	}
	if len(added) == 0 && len(removed) == 0 {
		return
	}
	sort.Slice(added, func(i, j int) bool { return added[i].AgentType < added[j].AgentType })
	sort.Strings(removed)
	addedTypes := make([]string, 0, len(added))
	for _, a := range added {
		addedTypes = append(addedTypes, a.AgentType)
	}
	e.appendMessages(e.attachmentReg.BuildMessages(attachments.Context{
		AgentListingDelta: &attachments.AgentListingDeltaAttachment{
			AddedTypes:          addedTypes,
			AddedLines:          agents.FormatLines(added),
			RemovedTypes:        removed,
			IsInitial:           len(announced) == 0,
			ShowConcurrencyNote: !envTruthy("CLAUDE_GO_SUBSCRIPTION_PRO"),
		},
	}))
}

func (e *QueryEngine) announceDeferredToolsDelta() {
	if e.exposure.allowedSet == nil {
		return
	}
	if !e.exposure.IsExposed("ToolSearch") {
		return
	}
	modelUsed := e.modelUsedForTurn()
	if !toolsearchutil.IsEnabled(modelUsed, e.exposedTools) {
		return
	}
	current := make([]string, 0, 64)
	poolNames := map[string]struct{}{}
	for _, t := range e.reg.List() {
		if !e.exposure.IsExposed(t.Name()) {
			continue
		}
		poolNames[t.Name()] = struct{}{}
		if toolsearchutil.IsDeferredTool(t) {
			current = append(current, t.Name())
		}
	}
	sort.Strings(current)
	announced := scanAnnouncedDeferredTools(e.msgs)
	currentSet := map[string]struct{}{}
	for _, n := range current {
		currentSet[n] = struct{}{}
	}
	added := make([]string, 0, len(current))
	for _, n := range current {
		if _, ok := announced[n]; !ok {
			added = append(added, n)
		}
	}
	removed := make([]string, 0, len(announced))
	for n := range announced {
		if _, ok := currentSet[n]; !ok {
			// TS: if a tool is no longer deferred but still exists in the base pool,
			// do not announce it as removed (it's now loaded inline).
			if _, stillInPool := poolNames[n]; stillInPool {
				continue
			}
			removed = append(removed, n)
		}
	}
	if len(added) == 0 && len(removed) == 0 {
		return
	}
	sort.Strings(removed)
	addedLines := make([]string, 0, len(added))
	for _, n := range added {
		addedLines = append(addedLines, "- "+n)
	}
	e.appendMessages(e.attachmentReg.BuildMessages(attachments.Context{
		DeferredToolsDelta: &attachments.DeferredToolsDeltaAttachment{
			AddedNames:   added,
			AddedLines:   addedLines,
			RemovedNames: removed,
		},
	}))
}

func (e *QueryEngine) appendMessage(msg api.Message) {
	e.msgs = append(e.msgs, msg)
	if e.transcript != nil {
		_ = e.transcript.Append([]api.Message{msg})
	}
}

func (e *QueryEngine) appendMessages(msgs []api.Message) {
	if len(msgs) == 0 {
		return
	}
	e.msgs = append(e.msgs, msgs...)
	if e.transcript != nil {
		_ = e.transcript.Append(msgs)
	}
}
