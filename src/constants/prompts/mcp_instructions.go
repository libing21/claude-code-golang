package prompts

import (
	"strings"
)

type MCPInstructionsClient struct {
	Name         string
	Instructions string
}

// BuildMcpInstructionsSection mirrors TS getMcpInstructions() formatting.
func BuildMcpInstructionsSection(clients []MCPInstructionsClient) string {
	blocks := make([]string, 0, len(clients))
	for _, c := range clients {
		if strings.TrimSpace(c.Instructions) == "" {
			continue
		}
		blocks = append(blocks, "## "+c.Name+"\n"+c.Instructions)
	}
	if len(blocks) == 0 {
		return ""
	}
	return "# MCP Server Instructions\n\nThe following MCP servers have provided instructions for how to use their tools and resources:\n\n" + strings.Join(blocks, "\n\n")
}

