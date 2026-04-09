package query

import (
	"fmt"
	"strings"

	"claude-code-running-go/src/services/api"
)

type fileWriteBatchStatus struct {
	Attempted  int
	Succeeded  int
	Failed     int
	FailSnippets []string
}

func isFileWriteToolUse(u api.ContentBlock) bool {
	switch u.Name {
	case "Write", "FileWrite", "FileEdit":
		return true
	case "Bash":
		m, ok := u.Input.(map[string]any)
		if !ok {
			return false
		}
		cmd, _ := m["command"].(string)
		cmd = strings.TrimSpace(cmd)
		// Rough heuristic: redirects/tee/cat heredoc usually indicate file write.
		if strings.Contains(cmd, " > ") || strings.Contains(cmd, ">>") || strings.Contains(cmd, "cat >") || strings.Contains(cmd, "tee ") {
			return true
		}
		return false
	default:
		return false
	}
}

func summarizeToolResult(r api.ToolResultBlock) string {
	s := fmt.Sprintf("%v", r.Content)
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) > 180 {
		s = s[:180] + "..."
	}
	return s
}

func computeFileWriteBatchStatus(toolUses []api.ContentBlock, toolResults []api.ToolResultBlock) fileWriteBatchStatus {
	st := fileWriteBatchStatus{}
	byID := map[string]api.ContentBlock{}
	for _, u := range toolUses {
		byID[u.ID] = u
	}
	for _, r := range toolResults {
		u, ok := byID[r.ToolUseID]
		if !ok || !isFileWriteToolUse(u) {
			continue
		}
		st.Attempted++
		if r.IsError {
			st.Failed++
			st.FailSnippets = append(st.FailSnippets, u.Name+": "+summarizeToolResult(r))
		} else {
			st.Succeeded++
		}
	}
	return st
}

func shouldBlockSuccessTone(text string) bool {
	t := strings.ToLower(text)
	for _, k := range []string{
		"已生成", "已创建", "已经为你生成", "文件被创建", "写入成功", "已保存", "已输出到",
	} {
		if strings.Contains(t, strings.ToLower(k)) {
			return true
		}
	}
	return false
}

