package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"claude-code-running-go/src/tool"
)

type Input struct {
	Query string `json:"query"`
	Num   int    `json:"num,omitempty"`
	LR    string `json:"lr,omitempty"`
}

type WebSearchTool struct{}

func New() *WebSearchTool { return &WebSearchTool{} }

func (t *WebSearchTool) Name() string { return "WebSearch" }

func (t *WebSearchTool) Prompt() string {
	return strings.TrimSpace(`- Searches the internet for relevant results
- Use sparingly; prefer local repo search tools when possible
- Returns a small list of results (title + url)`)
}

func (t *WebSearchTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type":"object",
  "additionalProperties":false,
  "properties":{
    "query":{"type":"string","description":"Search query"},
    "num":{"type":"integer","minimum":1,"maximum":10,"description":"Max results (optional, default 5)"},
    "lr":{"type":"string","description":"Language restriction (optional)"}
  },
  "required":["query"]
}`)
}

func (t *WebSearchTool) IsReadOnly(_ any) bool        { return true }
func (t *WebSearchTool) IsConcurrencySafe(_ any) bool { return true }

func (t *WebSearchTool) ValidateInput(input any) error {
	switch v := input.(type) {
	case Input:
		if strings.TrimSpace(v.Query) == "" {
			return fmt.Errorf("query is required")
		}
	}
	return nil
}

func (t *WebSearchTool) CheckPermissions(_ context.Context, _ any, _ tool.PermissionContext) (tool.PermissionDecision, any, error) {
	return tool.PermissionDecision{Behavior: tool.PermissionBehaviorAllow}, nil, nil
}

func (t *WebSearchTool) Call(ctx context.Context, input any) (tool.ToolResult, error) {
	var in Input
	switch v := input.(type) {
	case Input:
		in = v
	case map[string]any:
		b, _ := json.Marshal(v)
		if err := json.Unmarshal(b, &in); err != nil {
			return tool.ToolResult{IsError: true, Content: "invalid input"}, err
		}
	default:
		return tool.ToolResult{IsError: true, Content: "invalid input type"}, fmt.Errorf("invalid input type %T", input)
	}

	n := in.Num
	if n <= 0 {
		n = 5
	}
	if n > 10 {
		n = 10
	}

	// Best-effort: DuckDuckGo HTML endpoint (no API key). Intended for debugging.
	// If you need production-grade search, wire an API via env in a later step.
	q := url.QueryEscape(in.Query)
	searchURL := "https://duckduckgo.com/html/?q=" + q

	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return tool.ToolResult{IsError: true, Content: err.Error()}, nil
	}
	req.Header.Set("User-Agent", "claude-code-running-go/0.1")

	hc := &http.Client{Timeout: 30 * time.Second}
	resp, err := hc.Do(req)
	if err != nil {
		return tool.ToolResult{IsError: true, Content: err.Error()}, nil
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))

	// Parse a subset of results: anchor tags with class "result__a".
	re := regexp.MustCompile(`<a[^>]+class="[^"]*result__a[^"]*"[^>]+href="([^"]+)"[^>]*>(.*?)</a>`)
	matches := re.FindAllStringSubmatch(string(body), n)
	if len(matches) == 0 {
		return tool.ToolResult{Content: "No results found"}, nil
	}

	stripTags := regexp.MustCompile(`<[^>]+>`)
	var b strings.Builder
	for i, m := range matches {
		link := m[1]
		title := stripTags.ReplaceAllString(m[2], "")
		title = strings.TrimSpace(htmlUnescape(title))
		if title == "" {
			title = "(no title)"
		}
		fmt.Fprintf(&b, "%d. %s\n   %s\n", i+1, title, link)
	}
	return tool.ToolResult{Content: strings.TrimRight(b.String(), "\n")}, nil
}

func htmlUnescape(s string) string {
	r := strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", "\"",
		"&#39;", "'",
	)
	return r.Replace(s)
}

