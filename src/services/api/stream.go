package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// RawMessageStreamEvent mirrors TS BetaRawMessageStreamEvent shape loosely.
// We keep it flexible for phase-1: the only guaranteed field is "type".
// Consumers can switch on Type and inspect Payload for additional fields.
type RawMessageStreamEvent struct {
	Type    string
	Payload map[string]any
}

func (e *RawMessageStreamEvent) UnmarshalJSON(b []byte) error {
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	t, _ := m["type"].(string)
	e.Type = t
	e.Payload = m
	return nil
}

type MessageStream struct {
	Events    <-chan RawMessageStreamEvent
	Errors    <-chan error
	RequestID string
	Close     func() error
}

// CreateMessageStreaming is a convenience wrapper that consumes the stream and returns a full response.
// If onEvent is provided, it is called for every decoded event (useful for messages-dump).
func (c *Client) CreateMessageStreaming(ctx context.Context, in CreateMessageInput, onEvent func(RawMessageStreamEvent)) (*MessagesResponse, error) {
	ms, err := c.CreateMessageStream(ctx, in)
	if err != nil {
		return nil, err
	}
	defer ms.Close()

	acc := NewStreamAccumulator()

	eventsCh := ms.Events
	errCh := ms.Errors
	for eventsCh != nil || errCh != nil {
		select {
		case ev, ok := <-eventsCh:
			if !ok {
				eventsCh = nil
				continue
			}
			if onEvent != nil {
				onEvent(ev)
			}
			if err := acc.Consume(ev); err != nil {
				return nil, err
			}
		case e, ok := <-errCh:
			if !ok {
				errCh = nil
				continue
			}
			if e != nil && !errorsIsContextCanceled(e) {
				return nil, e
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return acc.Response(), nil
}

// CreateMessageStream starts a streaming Messages API request (stream=true) and emits SSE events.
// This is intentionally minimal and is meant to be wired into QueryEngine in the next step.
func (c *Client) CreateMessageStream(ctx context.Context, in CreateMessageInput) (*MessageStream, error) {
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
		Model:        model,
		MaxTokens:    maxTokens,
		System:       strings.Join(in.SystemPrompt, "\n\n"),
		Messages:     in.Messages,
		Tools:        in.Tools,
		OutputFormat: in.OutputFormat,
		Stream:       true,
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
	req.Header.Set("Accept", "text/event-stream")
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
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, fmt.Errorf("api error: %s: %s", resp.Status, string(body))
	}

	eventsCh := make(chan RawMessageStreamEvent, 64)
	errCh := make(chan error, 1)

	closeFn := func() error { return resp.Body.Close() }
	reqID := resp.Header.Get("request-id")
	if reqID == "" {
		// Best-effort: different proxies/providers use different headers.
		reqID = resp.Header.Get("anthropic-request-id")
	}

	go func() {
		defer close(eventsCh)
		defer close(errCh)
		defer resp.Body.Close()

		if err := readSSE(ctx, resp.Body, func(_ string, data []byte) error {
			var ev RawMessageStreamEvent
			if err := json.Unmarshal(data, &ev); err != nil {
				return fmt.Errorf("decode stream event: %w", err)
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case eventsCh <- ev:
				return nil
			}
		}); err != nil && !errorsIsContextCanceled(err) {
			errCh <- err
		}
	}()

	return &MessageStream{
		Events:    eventsCh,
		Errors:    errCh,
		RequestID: reqID,
		Close:     closeFn,
	}, nil
}

func errorsIsContextCanceled(err error) bool {
	return err == context.Canceled || err == context.DeadlineExceeded
}

// readSSE parses a subset of Server-Sent Events: it only cares about "event:" and "data:" fields.
// Records are separated by a blank line. "data:" lines are concatenated with "\n".
func readSSE(ctx context.Context, r io.Reader, onEvent func(eventName string, data []byte) error) error {
	br := bufio.NewReader(r)

	var eventName string
	var dataBuf strings.Builder

	flush := func() error {
		if dataBuf.Len() == 0 {
			eventName = ""
			return nil
		}
		data := dataBuf.String()
		// remove final '\n' introduced by concatenating data lines
		data = strings.TrimSuffix(data, "\n")
		dataBuf.Reset()
		en := eventName
		eventName = ""

		// Some SSE servers use [DONE] sentinel; ignore if present.
		if strings.TrimSpace(data) == "[DONE]" {
			return io.EOF
		}
		return onEvent(en, []byte(data))
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line, err := br.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				// flush pending data if any; treat clean EOF as success.
				if ferr := flush(); ferr != nil && ferr != io.EOF {
					return ferr
				}
				return nil
			}
			return err
		}
		line = strings.TrimRight(line, "\r\n")

		if line == "" {
			if err := flush(); err != nil {
				if err == io.EOF {
					return nil
				}
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if rest, ok := strings.CutPrefix(line, "event:"); ok {
			eventName = strings.TrimSpace(rest)
			continue
		}
		if rest, ok := strings.CutPrefix(line, "data:"); ok {
			dataBuf.WriteString(strings.TrimSpace(rest))
			dataBuf.WriteString("\n")
			continue
		}
	}
}
