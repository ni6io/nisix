package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type transportKind string

const (
	transportStdio          transportKind = "stdio"
	transportSSE            transportKind = "sse"
	transportStreamableHTTP transportKind = "streamable_http"
)

type framingKind int

const (
	framingLSP framingKind = iota
	framingNDJSON
)

type Notification struct {
	Server string          `json:"server"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type Client struct {
	name      string
	logger    *slog.Logger
	transport transportKind
	framing   framingKind

	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	httpClient     *http.Client
	requestTimeout time.Duration
	url            string
	headers        map[string]string

	sseURL        string
	sseMessageURL string
	sseBody       io.ReadCloser
	sseReady      chan string

	writeMu       sync.Mutex
	pendingMu     sync.Mutex
	pending       map[string]chan rpcResponse
	notifications chan Notification

	closeOnce sync.Once
	closedCh  chan struct{}
	readErr   error
}

type RemoteTool struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

type rpcResponse struct {
	result json.RawMessage
	err    error
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

func StartClient(ctx context.Context, name string, baseDir string, cfg ServerConfig, logger *slog.Logger) (*Client, error) {
	if logger == nil {
		logger = slog.Default()
	}
	kind, err := resolveTransport(cfg)
	if err != nil {
		return nil, fmt.Errorf("mcp: server %s: %w", name, err)
	}

	framing := resolveFraming(cfg)

	timeoutSec := cfg.TimeoutSec
	if timeoutSec <= 0 {
		timeoutSec = 60
	}

	client := &Client{
		name:           name,
		logger:         logger,
		transport:      kind,
		framing:        framing,
		headers:        cloneStringMap(cfg.Headers),
		pending:        make(map[string]chan rpcResponse),
		notifications:  make(chan Notification, 128),
		closedCh:       make(chan struct{}),
		sseReady:       make(chan string, 1),
		requestTimeout: time.Duration(timeoutSec) * time.Second,
		// Keep transport-level timeout disabled for long-lived streams (SSE).
		httpClient: &http.Client{},
	}

	switch kind {
	case transportStdio:
		if err := client.startStdio(ctx, baseDir, cfg); err != nil {
			return nil, err
		}
	case transportSSE:
		if err := client.startSSE(ctx, cfg); err != nil {
			_ = client.Close()
			return nil, err
		}
	case transportStreamableHTTP:
		client.url = strings.TrimSpace(cfg.URL)
		if client.url == "" {
			return nil, fmt.Errorf("mcp: server %s streamable_http url is empty", name)
		}
	default:
		return nil, fmt.Errorf("mcp: server %s unsupported transport %q", name, kind)
	}

	if err := client.initialize(ctx); err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

func (c *Client) Notifications() <-chan Notification {
	if c == nil {
		return nil
	}
	return c.notifications
}

func (c *Client) Done() <-chan struct{} {
	if c == nil {
		return nil
	}
	return c.closedCh
}

func (c *Client) startStdio(ctx context.Context, baseDir string, cfg ServerConfig) error {
	command := strings.TrimSpace(cfg.Command)
	if command == "" {
		return fmt.Errorf("mcp: server %s command is empty", c.name)
	}

	cmd := exec.CommandContext(ctx, command, cfg.Args...)
	cmd.Dir = resolveDir(baseDir, cfg.Cwd)
	cmd.Env = mergeEnv(cfg.Env)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("mcp: server %s stdin: %w", c.name, err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("mcp: server %s stdout: %w", c.name, err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("mcp: server %s stderr: %w", c.name, err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("mcp: server %s start: %w", c.name, err)
	}

	c.cmd = cmd
	c.stdin = stdin
	c.stdout = stdout
	c.stderr = stderr

	if c.framing == framingNDJSON {
		go c.readLoopStdioNDJSON()
	} else {
		go c.readLoopStdio()
	}
	go c.stderrLoop()
	go c.waitLoop()
	return nil
}

func (c *Client) startSSE(ctx context.Context, cfg ServerConfig) error {
	c.sseURL = strings.TrimSpace(cfg.URL)
	if c.sseURL == "" {
		return fmt.Errorf("mcp: server %s sse url is empty", c.name)
	}
	if v := strings.TrimSpace(cfg.MessageURL); v != "" {
		c.sseMessageURL = resolveURL(c.sseURL, v)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.sseURL, nil)
	if err != nil {
		return fmt.Errorf("mcp: server %s sse request: %w", c.name, err)
	}
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("mcp: server %s sse connect: %w", c.name, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("mcp: server %s sse status %d: %s", c.name, resp.StatusCode, strings.TrimSpace(string(body)))
	}

	c.sseBody = resp.Body
	go c.readLoopSSE(resp.Body)

	if c.sseMessageURL == "" {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(8 * time.Second):
			return fmt.Errorf("mcp: server %s sse endpoint discovery timeout", c.name)
		case endpoint := <-c.sseReady:
			c.sseMessageURL = endpoint
		}
	}
	return nil
}

func (c *Client) ListTools(ctx context.Context) ([]RemoteTool, error) {
	if c == nil {
		return nil, fmt.Errorf("mcp: client is nil")
	}
	out := make([]RemoteTool, 0)
	cursor := ""
	for {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		result, err := c.request(ctx, "tools/list", params)
		if err != nil {
			return nil, err
		}
		var payload struct {
			Tools      []RemoteTool `json:"tools"`
			NextCursor string       `json:"nextCursor"`
		}
		if err := json.Unmarshal(result, &payload); err != nil {
			return nil, fmt.Errorf("mcp: server %s tools/list decode: %w", c.name, err)
		}
		out = append(out, payload.Tools...)
		if strings.TrimSpace(payload.NextCursor) == "" {
			break
		}
		cursor = strings.TrimSpace(payload.NextCursor)
	}
	return out, nil
}

func (c *Client) CallTool(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
	if c == nil {
		return nil, fmt.Errorf("mcp: client is nil")
	}
	if args == nil {
		args = map[string]any{}
	}
	result, err := c.request(ctx, "tools/call", map[string]any{
		"name":      name,
		"arguments": args,
	})
	if err != nil {
		return nil, err
	}
	var payload map[string]any
	if err := json.Unmarshal(result, &payload); err != nil {
		return nil, fmt.Errorf("mcp: server %s tools/call decode: %w", c.name, err)
	}
	if isErr, _ := payload["isError"].(bool); isErr {
		return nil, errors.New(extractCallError(payload))
	}

	out := make(map[string]any)
	if sc, ok := payload["structuredContent"]; ok {
		out["structuredContent"] = sc
	}
	if content, ok := payload["content"]; ok {
		out["content"] = content
	}
	if len(out) == 0 {
		for k, v := range payload {
			out[k] = v
		}
	}
	return out, nil
}

func (c *Client) Close() error {
	if c == nil {
		return nil
	}
	c.closeOnce.Do(func() {
		close(c.closedCh)
		if c.stdin != nil {
			_ = c.stdin.Close()
		}
		if c.stdout != nil {
			_ = c.stdout.Close()
		}
		if c.stderr != nil {
			_ = c.stderr.Close()
		}
		if c.sseBody != nil {
			_ = c.sseBody.Close()
		}
		if c.cmd != nil && c.cmd.Process != nil {
			_ = c.cmd.Process.Kill()
		}
		c.pendingMu.Lock()
		for id, ch := range c.pending {
			ch <- rpcResponse{err: errors.New("mcp client closed")}
			close(ch)
			delete(c.pending, id)
		}
		c.pendingMu.Unlock()
	})
	return nil
}

func (c *Client) initialize(ctx context.Context) error {
	result, err := c.request(ctx, "initialize", map[string]any{
		"protocolVersion": "2024-11-05",
		"capabilities":    map[string]any{},
		"clientInfo": map[string]any{
			"name":    "nisix",
			"version": "0.1.0",
		},
	})
	if err != nil {
		return err
	}
	if len(result) == 0 {
		return fmt.Errorf("mcp: server %s initialize returned empty result", c.name)
	}
	if err := c.notify("notifications/initialized", map[string]any{}); err != nil {
		return err
	}
	return nil
}

func (c *Client) notify(method string, params any) error {
	msg := map[string]any{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	switch c.transport {
	case transportStdio:
		return c.writeStdioMessage(msg)
	case transportSSE:
		_, err := c.postJSONRPC(context.Background(), c.sseMessageURL, msg, false)
		return err
	case transportStreamableHTTP:
		_, err := c.postJSONRPC(context.Background(), c.url, msg, false)
		return err
	default:
		return fmt.Errorf("mcp: unsupported transport %q", c.transport)
	}
}

func (c *Client) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	id := strconv.FormatInt(nextID(), 10)
	msg := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
		"params":  params,
	}

	switch c.transport {
	case transportStdio:
		return c.requestAsync(ctx, id, msg, c.writeStdioMessage)
	case transportSSE:
		return c.requestSSE(ctx, id, msg)
	case transportStreamableHTTP:
		return c.requestHTTP(ctx, msg)
	default:
		return nil, fmt.Errorf("mcp: unsupported transport %q", c.transport)
	}
}

func (c *Client) requestSSE(ctx context.Context, id string, msg map[string]any) (json.RawMessage, error) {
	respCh := make(chan rpcResponse, 1)
	c.pendingMu.Lock()
	c.pending[id] = respCh
	c.pendingMu.Unlock()

	result, err := c.postJSONRPC(ctx, c.sseMessageURL, msg, true)
	if err == nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return result, nil
	}
	if !errors.Is(err, errNoAsyncResponse) {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, err
	}
	return c.awaitPending(ctx, id, respCh)
}

func (c *Client) requestAsync(ctx context.Context, id string, msg map[string]any, writeFn func(map[string]any) error) (json.RawMessage, error) {
	respCh := make(chan rpcResponse, 1)
	c.pendingMu.Lock()
	c.pending[id] = respCh
	c.pendingMu.Unlock()

	if err := writeFn(msg); err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, err
	}

	return c.awaitPending(ctx, id, respCh)
}

func (c *Client) awaitPending(ctx context.Context, id string, respCh <-chan rpcResponse) (json.RawMessage, error) {
	select {
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, ctx.Err()
	case <-c.closedCh:
		return nil, errors.New("mcp client closed")
	case resp := <-respCh:
		if resp.err != nil {
			return nil, resp.err
		}
		return resp.result, nil
	}
}

func (c *Client) requestHTTP(ctx context.Context, msg map[string]any) (json.RawMessage, error) {
	return c.postJSONRPC(ctx, c.url, msg, true)
}

var errNoAsyncResponse = errors.New("no async response")

func (c *Client) postJSONRPC(ctx context.Context, endpoint string, msg map[string]any, expectResult bool) (json.RawMessage, error) {
	if strings.TrimSpace(endpoint) == "" {
		return nil, fmt.Errorf("mcp: endpoint is empty")
	}
	body, err := json.Marshal(msg)
	if err != nil {
		return nil, err
	}

	reqCtx, cancel := c.withRequestTimeout(ctx)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range c.headers {
		req.Header.Set(k, v)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("mcp: server %s status %d: %s", c.name, resp.StatusCode, strings.TrimSpace(string(raw)))
	}

	contentType := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if strings.Contains(contentType, "text/event-stream") {
		if !expectResult {
			return nil, nil
		}
		requestID, ok := normalizeID(msg["id"])
		if !ok || strings.TrimSpace(requestID) == "" {
			return nil, errors.New("json-rpc request id is missing")
		}
		return readEventStreamResult(reqCtx, resp.Body, requestID, c.emitNotification)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		if expectResult {
			return nil, errNoAsyncResponse
		}
		return nil, nil
	}
	result, parseErr := parseJSONRPCResult(raw)
	if parseErr == nil {
		return result, nil
	}
	if !expectResult {
		return nil, nil
	}
	return json.RawMessage(raw), nil
}

func parseJSONRPCResult(raw []byte) (json.RawMessage, error) {
	var msg rpcMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil, err
	}
	if msg.Error != nil {
		return nil, fmt.Errorf("mcp rpc error %d: %s", msg.Error.Code, msg.Error.Message)
	}
	if len(msg.Result) == 0 {
		return nil, errors.New("json-rpc result is empty")
	}
	return msg.Result, nil
}

func (c *Client) withRequestTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.requestTimeout <= 0 {
		return ctx, func() {}
	}
	if _, ok := ctx.Deadline(); ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, c.requestTimeout)
}

func readEventStreamResult(
	ctx context.Context,
	stream io.Reader,
	requestID string,
	onNotification func(method string, params json.RawMessage),
) (json.RawMessage, error) {
	reader := bufio.NewReader(stream)
	event := ""
	dataLines := make([]string, 0, 1)

	flush := func() (json.RawMessage, bool, error) {
		if len(dataLines) == 0 {
			return nil, false, nil
		}
		data := strings.TrimSpace(strings.Join(dataLines, "\n"))
		dataLines = dataLines[:0]
		if data == "" {
			return nil, false, nil
		}
		if strings.EqualFold(event, "endpoint") || (event == "" && !strings.HasPrefix(data, "{")) {
			return nil, false, nil
		}
		var msg rpcMessage
		if err := json.Unmarshal([]byte(data), &msg); err != nil {
			return nil, false, nil
		}
		if msg.ID == nil {
			if strings.TrimSpace(msg.Method) != "" {
				onNotification(msg.Method, msg.Params)
			}
			return nil, false, nil
		}
		id, ok := normalizeID(msg.ID)
		if !ok || id != requestID {
			return nil, false, nil
		}
		if msg.Error != nil {
			return nil, true, fmt.Errorf("mcp rpc error %d: %s", msg.Error.Code, msg.Error.Message)
		}
		if len(msg.Result) == 0 {
			return nil, true, errors.New("json-rpc result is empty")
		}
		return msg.Result, true, nil
	}

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		line, err := reader.ReadString('\n')
		if err != nil {
			if errors.Is(err, io.EOF) {
				if result, found, ferr := flush(); found || ferr != nil {
					return result, ferr
				}
				return nil, errNoAsyncResponse
			}
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if result, found, ferr := flush(); found || ferr != nil {
				return result, ferr
			}
			event = ""
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
}

func (c *Client) emitNotification(method string, params json.RawMessage) {
	method = strings.TrimSpace(method)
	if method == "" {
		return
	}
	note := Notification{Server: c.name, Method: method}
	if len(params) > 0 {
		note.Params = append(json.RawMessage(nil), params...)
	}
	select {
	case <-c.closedCh:
		return
	case c.notifications <- note:
		return
	default:
		if c.logger != nil {
			c.logger.Warn("mcp.notification.drop", "server", c.name, "method", method)
		}
	}
}

func (c *Client) writeStdioMessage(msg map[string]any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.stdin == nil {
		return errors.New("mcp stdin is nil")
	}

	body, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	switch c.framing {
	case framingNDJSON:
		if _, err := c.stdin.Write(append(body, '\n')); err != nil {
			return err
		}
		return nil
	default:
		var frame bytes.Buffer
		_, _ = fmt.Fprintf(&frame, "Content-Length: %d\r\n\r\n", len(body))
		frame.Write(body)
		_, err = c.stdin.Write(frame.Bytes())
		return err
	}
}

func (c *Client) readLoopStdio() {
	reader := bufio.NewReader(c.stdout)
	for {
		body, err := readFrame(reader)
		if err != nil {
			c.failPending(err)
			return
		}
		c.dispatchRPCMessage(body)
	}
}

func (c *Client) readLoopStdioNDJSON() {
	scanner := bufio.NewScanner(c.stdout)
	buf := make([]byte, 0, 1024*128)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		c.dispatchRPCMessage([]byte(line))
	}
	if err := scanner.Err(); err != nil {
		c.failPending(err)
		return
	}
	c.failPending(io.EOF)
}

func (c *Client) readLoopSSE(stream io.Reader) {
	reader := bufio.NewReader(stream)
	event := ""
	dataLines := make([]string, 0, 1)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			c.failPending(err)
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			c.handleSSEEvent(event, strings.Join(dataLines, "\n"))
			event = ""
			dataLines = dataLines[:0]
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
}

func (c *Client) handleSSEEvent(event string, data string) {
	if strings.TrimSpace(data) == "" {
		return
	}
	if strings.EqualFold(event, "endpoint") || (event == "" && !strings.HasPrefix(strings.TrimSpace(data), "{")) {
		endpoint := resolveURL(c.sseURL, strings.TrimSpace(data))
		select {
		case c.sseReady <- endpoint:
		default:
		}
		return
	}
	c.dispatchRPCMessage([]byte(data))
}

func (c *Client) dispatchRPCMessage(body []byte) {
	var msg rpcMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		c.logger.Warn("mcp.response.decode.error", "server", c.name, "err", err)
		return
	}
	if msg.ID == nil {
		if strings.TrimSpace(msg.Method) != "" {
			c.emitNotification(msg.Method, msg.Params)
		}
		return
	}
	id, ok := normalizeID(msg.ID)
	if !ok {
		return
	}

	c.pendingMu.Lock()
	respCh, exists := c.pending[id]
	if exists {
		delete(c.pending, id)
	}
	c.pendingMu.Unlock()
	if !exists {
		return
	}

	if msg.Error != nil {
		respCh <- rpcResponse{err: fmt.Errorf("mcp: server %s rpc error %d: %s", c.name, msg.Error.Code, msg.Error.Message)}
		close(respCh)
		return
	}
	respCh <- rpcResponse{result: msg.Result}
	close(respCh)
}

func (c *Client) stderrLoop() {
	scanner := bufio.NewScanner(c.stderr)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		c.logger.Warn("mcp.server.stderr", "server", c.name, "line", line)
	}
}

func (c *Client) waitLoop() {
	if c.cmd == nil {
		return
	}
	if err := c.cmd.Wait(); err != nil {
		c.failPending(err)
		return
	}
	c.failPending(io.EOF)
}

func (c *Client) failPending(err error) {
	if err == nil {
		return
	}
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	if c.readErr == nil {
		c.readErr = err
	}
	for id, ch := range c.pending {
		ch <- rpcResponse{err: err}
		close(ch)
		delete(c.pending, id)
	}
}

func readFrame(reader *bufio.Reader) ([]byte, error) {
	contentLength := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(parts[0]), "Content-Length") {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, fmt.Errorf("invalid content length: %w", err)
		}
		contentLength = n
	}
	if contentLength < 0 {
		return nil, errors.New("missing Content-Length")
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, body); err != nil {
		return nil, err
	}
	return body, nil
}

func resolveTransport(cfg ServerConfig) (transportKind, error) {
	transport := strings.ToLower(strings.TrimSpace(cfg.Transport))
	if transport == "" {
		transport = strings.ToLower(strings.TrimSpace(cfg.Type))
	}
	if transport == "" {
		if strings.TrimSpace(cfg.Command) != "" {
			return transportStdio, nil
		}
		if strings.TrimSpace(cfg.URL) != "" {
			if strings.TrimSpace(cfg.MessageURL) != "" {
				return transportSSE, nil
			}
			if strings.Contains(strings.ToLower(cfg.URL), "/sse") {
				return transportSSE, nil
			}
			return transportStreamableHTTP, nil
		}
		return "", errors.New("transport is not set and neither command nor url is provided")
	}
	switch transport {
	case string(transportStdio):
		return transportStdio, nil
	case string(transportSSE):
		return transportSSE, nil
	case string(transportStreamableHTTP), "streamable-http", "http":
		return transportStreamableHTTP, nil
	default:
		return "", fmt.Errorf("unsupported transport %q", transport)
	}
}

func resolveFraming(cfg ServerConfig) framingKind {
	f := strings.ToLower(strings.TrimSpace(cfg.Framing))
	switch f {
	case "ndjson", "newline", "jsonlines", "jsonl":
		return framingNDJSON
	case "lsp", "content_length", "content-length":
		return framingLSP
	}

	command := strings.ToLower(strings.TrimSpace(cfg.Command))
	argsJoined := strings.ToLower(strings.Join(cfg.Args, " "))
	if strings.Contains(command, "server-filesystem") || strings.Contains(argsJoined, "server-filesystem") || strings.Contains(command, "mcp-server-filesystem") {
		return framingNDJSON
	}

	return framingLSP
}

func resolveDir(baseDir string, configured string) string {
	configured = strings.TrimSpace(configured)
	if configured == "" {
		return baseDir
	}
	if filepath.IsAbs(configured) {
		return configured
	}
	return filepath.Join(baseDir, configured)
}

func mergeEnv(extra map[string]string) []string {
	base := os.Environ()
	if len(extra) == 0 {
		return base
	}
	keys := make([]string, 0, len(extra))
	for k := range extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		base = append(base, k+"="+extra[k])
	}
	return base
}

func extractCallError(payload map[string]any) string {
	if content, ok := payload["content"].([]any); ok {
		parts := make([]string, 0, len(content))
		for _, item := range content {
			entry, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if text, ok := entry["text"].(string); ok && strings.TrimSpace(text) != "" {
				parts = append(parts, text)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	}
	return "mcp tool call failed"
}

func cloneStringMap(in map[string]string) map[string]string {
	if in == nil {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func resolveURL(baseURL string, endpoint string) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return strings.TrimSpace(baseURL)
	}
	if parsed, err := neturl.Parse(endpoint); err == nil && parsed.IsAbs() {
		return parsed.String()
	}
	base, err := neturl.Parse(strings.TrimSpace(baseURL))
	if err != nil {
		return endpoint
	}
	ref, err := neturl.Parse(endpoint)
	if err != nil {
		return endpoint
	}
	return base.ResolveReference(ref).String()
}

var (
	idMu      sync.Mutex
	idCounter int64
)

func nextID() int64 {
	idMu.Lock()
	defer idMu.Unlock()
	idCounter++
	return idCounter
}

func normalizeID(v any) (string, bool) {
	switch id := v.(type) {
	case string:
		return id, true
	case float64:
		return strconv.FormatInt(int64(id), 10), true
	case json.Number:
		return id.String(), true
	default:
		return "", false
	}
}
