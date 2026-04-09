package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
	"time"
)

type rpcRequest struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int64  `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      int64           `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type Client struct {
	name string

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser

	nextID  atomic.Int64
	pending sync.Map // id -> chan rpcResponse

	writeMu sync.Mutex

	doneOnce sync.Once
	doneCh   chan struct{}

	// Initialized server fields (best-effort, for prompt parity with TS).
	instructions string
}

func StartServer(name string, cfg ServerConfig) (*Client, error) {
	cmd := exec.Command(cfg.Command, cfg.Args...)
	env := make([]string, 0)
	// inherit env by default; then add overrides
	env = append(env, cmd.Environ()...)
	for k, v := range cfg.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = env

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	// Merge stderr into stdout for easier debugging; TS keeps separate but we
	// just want deterministic print-mode behavior.
	cmd.Stderr = cmd.Stdout

	if err := cmd.Start(); err != nil {
		return nil, err
	}

	c := &Client{
		name:   name,
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
		doneCh: make(chan struct{}),
	}
	go c.readLoop()
	return c, nil
}

func (c *Client) Close() error {
	var err error
	c.doneOnce.Do(func() {
		close(c.doneCh)
		_ = c.stdin.Close()
		// best-effort terminate
		_ = c.cmd.Process.Kill()
		_, err = c.cmd.Process.Wait()
	})
	return err
}

func (c *Client) readLoop() {
	defer c.Close()
	sc := bufio.NewScanner(c.stdout)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		var resp rpcResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			// ignore non-json lines; MCP servers may log.
			continue
		}
		if resp.ID == 0 {
			continue
		}
		if ch, ok := c.pending.Load(resp.ID); ok {
			rc := ch.(chan rpcResponse)
			select {
			case rc <- resp:
			default:
			}
		}
	}
}

func (c *Client) Call(ctx context.Context, method string, params any, out any) error {
	id := c.nextID.Add(1)
	req := rpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	b, err := json.Marshal(req)
	if err != nil {
		return err
	}
	b = append(b, '\n')

	ch := make(chan rpcResponse, 1)
	c.pending.Store(id, ch)
	defer c.pending.Delete(id)

	c.writeMu.Lock()
	_, err = c.stdin.Write(b)
	c.writeMu.Unlock()
	if err != nil {
		return err
	}

	timeout := 30 * time.Second
	if dl, ok := ctx.Deadline(); ok {
		if d := time.Until(dl); d > 0 {
			timeout = d
		}
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.doneCh:
		return fmt.Errorf("mcp server %s closed", c.name)
	case <-timer.C:
		return fmt.Errorf("mcp call timeout: %s", method)
	case resp := <-ch:
		if resp.Error != nil {
			return fmt.Errorf("mcp error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		if out == nil {
			return nil
		}
		return json.Unmarshal(resp.Result, out)
	}
}

type rpcNotification struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

func (c *Client) Notify(ctx context.Context, method string, params any) error {
	// Notifications have no ID and no response is expected.
	_ = ctx
	msg := rpcNotification{JSONRPC: "2.0", Method: method, Params: params}
	b, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	c.writeMu.Lock()
	_, err = c.stdin.Write(b)
	c.writeMu.Unlock()
	return err
}

func (c *Client) setInstructions(s string) {
	c.instructions = s
}

func (c *Client) Instructions() string {
	return c.instructions
}
