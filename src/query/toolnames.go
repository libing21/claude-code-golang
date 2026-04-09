package query

import (
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
)

// GetDefaultToolNamesForPrompt returns the tool names exposed in the default Go --print session.
// This is used for building a TS-like system prompt that depends on enabled tools.
func GetDefaultToolNamesForPrompt(skillDirs []string) []string {
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
		discoverskills.New(skillDirs),
		agent.New(),
		skill.New(skillDirs),
	})
	reg.Add(toolsearchtool.New(func() []tool.Tool {
		out := make([]tool.Tool, 0, 32)
		for _, tt := range reg.List() {
			if mt, ok := tt.(interface{ IsMCPTool() bool }); ok && mt.IsMCPTool() {
				out = append(out, tt)
			}
		}
		return out
	}))
	names := make([]string, 0, 16)
	for _, t := range reg.List() {
		names = append(names, t.Name())
	}
	return names
}
