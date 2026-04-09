package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ClientConfig is a minimal subset for phase-1.
type ClientConfig struct {
	BaseURL   string
	Model     string
	APIKey    string
	AuthToken string
	Timeout   time.Duration
}

type Client struct {
	cfg ClientConfig
	hc  *http.Client
}

func (c *Client) Model() string { return c.cfg.Model }

func NewClient(cfg ClientConfig) *Client {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 10 * time.Minute
	}
	return &Client{
		cfg: cfg,
		hc:  &http.Client{Timeout: timeout},
	}
}

// MessagesRequest/Response implements a minimal Anthropic-compatible Messages API.
// We keep types local for easy debugging and later 1:1 expansion.
type MessagesRequest struct {
	Model     string           `json:"model"`
	MaxTokens int              `json:"max_tokens"`
	System    string           `json:"system,omitempty"`
	Messages  []Message        `json:"messages"`
	Tools     []ToolSchema      `json:"tools,omitempty"`
	OutputFormat any           `json:"output_format,omitempty"`
	Stream    bool             `json:"stream,omitempty"`
}

type Message struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

type ToolSchema struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

type MessagesResponse struct {
	ID         string         `json:"id"`
	Type       string         `json:"type"`
	Role       string         `json:"role"`
	Content    []ContentBlock `json:"content"`
	StopReason string         `json:"stop_reason"`
}

type ContentBlock struct {
	Type  string `json:"type"`
	Text  string `json:"text,omitempty"`
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Input any    `json:"input,omitempty"`
}

type ToolResultBlock struct {
	Type      string `json:"type"` // "tool_result"
	ToolUseID string `json:"tool_use_id"`
	Content   any    `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

type CreateMessageInput struct {
	SystemPrompt []string
	Messages     []Message
	Tools        []ToolSchema
	// Optional overrides for side queries and TS parity.
	Model        string
	MaxTokens    int
	OutputFormat any
}

func (c *Client) CreateMessage(ctx context.Context, in CreateMessageInput) (*MessagesResponse, error) {
	url := strings.TrimRight(c.cfg.BaseURL, "/") + "/v1/messages"
	model := c.cfg.Model
	if strings.TrimSpace(in.Model) != "" {
		model = strings.TrimSpace(in.Model)
	}
	maxTokens := 2048
	if in.MaxTokens > 0 {
		maxTokens = in.MaxTokens
	}
	reqBody := MessagesRequest{
		Model:     model,
		MaxTokens: maxTokens,
		System:    strings.Join(in.SystemPrompt, "\n\n"),
		Messages:  in.Messages,
		Tools:     in.Tools,
		OutputFormat: in.OutputFormat,
	}
	b, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("anthropic-version", "2023-06-01")
	if c.cfg.APIKey != "" {
		req.Header.Set("x-api-key", c.cfg.APIKey)
	} else if c.cfg.AuthToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.cfg.AuthToken)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("api error: %s: %s", resp.Status, string(body))
	}
	var out MessagesResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode response: %w; body=%s", err, string(body))
	}
	return &out, nil
}
