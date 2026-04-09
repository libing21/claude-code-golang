package webfetch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"claude-code-running-go/src/tool"
	"golang.org/x/net/html"
)

type Input struct {
	URL string `json:"url"`
}

type WebFetchTool struct{}

func New() *WebFetchTool { return &WebFetchTool{} }

func (t *WebFetchTool) Name() string { return "WebFetch" }

func (t *WebFetchTool) Prompt() string {
	return strings.TrimSpace(`- Fetches a URL over HTTP(S)
- Returns a text representation; HTML is converted to plain text (best-effort)
- Use when you need to retrieve web content for analysis`)
}

func (t *WebFetchTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{
  "type":"object",
  "additionalProperties":false,
  "properties":{
    "url":{"type":"string","description":"URL to fetch","format":"uri"}
  },
  "required":["url"]
}`)
}

func (t *WebFetchTool) IsReadOnly(_ any) bool        { return true }
func (t *WebFetchTool) IsConcurrencySafe(_ any) bool { return true }

func (t *WebFetchTool) ValidateInput(input any) error {
	switch v := input.(type) {
	case Input:
		if strings.TrimSpace(v.URL) == "" {
			return fmt.Errorf("url is required")
		}
	}
	return nil
}

func (t *WebFetchTool) CheckPermissions(_ context.Context, _ any, _ tool.PermissionContext) (tool.PermissionDecision, any, error) {
	// Read-only but potentially sensitive; keep it allow for now, user can disallow via rules.
	return tool.PermissionDecision{Behavior: tool.PermissionBehaviorAllow}, nil, nil
}

func textFromHTML(r io.Reader) (string, error) {
	doc, err := html.Parse(r)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	var f func(n *html.Node)
	f = func(n *html.Node) {
		if n.Type == html.TextNode {
			t := strings.TrimSpace(n.Data)
			if t != "" {
				b.WriteString(t)
				b.WriteString("\n")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)
	return strings.TrimSpace(b.String()), nil
}

func (t *WebFetchTool) Call(ctx context.Context, input any) (tool.ToolResult, error) {
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

	u, err := url.Parse(in.URL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return tool.ToolResult{IsError: true, Content: "invalid url"}, nil
	}
	if u.Scheme == "http" {
		u.Scheme = "https"
	}

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
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
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	var out string
	if strings.Contains(ct, "text/html") {
		out, err = textFromHTML(strings.NewReader(string(body)))
		if err != nil {
			out = string(body)
		}
	} else {
		out = string(body)
	}

	if out == "" {
		out = "(empty response body)"
	}
	return tool.ToolResult{Content: out}, nil
}

