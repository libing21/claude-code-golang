package attachments

import (
	"strings"

	"claude-code-running-go/src/services/api"
)

const KindMcpInstructionsDelta Kind = "mcp_instructions_delta"

// MCP instructions delta provider.
// TS equivalent: getMcpInstructionsDelta()/isMcpInstructionsDeltaEnabled() plumbing.
type McpInstructionsDeltaProvider struct{}

func (p McpInstructionsDeltaProvider) Kind() Kind    { return KindMcpInstructionsDelta }
func (p McpInstructionsDeltaProvider) Priority() int { return 100 }

func (p McpInstructionsDeltaProvider) Build(ctx Context) []api.Message {
	if !ctx.McpInstructionsDelta {
		return nil
	}
	sec := strings.TrimSpace(ctx.McpInstructionsSection)
	if sec == "" {
		return nil
	}
	content := "<mcp_instructions_delta>\n" + sec + "\n</mcp_instructions_delta>"
	return []api.Message{{Role: "user", Content: content}}
}

