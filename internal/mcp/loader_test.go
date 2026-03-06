package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ni6io/nisix/internal/tools"
)

const helperEnv = "NISIX_MCP_TEST_HELPER"

func TestRegisterFromFileRegistersAndExecutesTools_Stdio(t *testing.T) {
	if os.Getenv(helperEnv) == "1" {
		runHelperServer()
		os.Exit(0)
	}

	dir := t.TempDir()
	configPath := filepath.Join(dir, "mcp.json")
	cfg := FileConfig{
		MCPServers: map[string]ServerConfig{
			"demo": {
				Transport: "stdio",
				Command:   os.Args[0],
				Args:      []string{"-test.run=TestRegisterFromFileRegistersAndExecutesTools_Stdio", "--"},
				Env: map[string]string{
					helperEnv: "1",
				},
			},
		},
	}
	writeMCPConfig(t, configPath, cfg)

	reg := tools.NewRegistry()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	manager, count, err := RegisterFromFile(ctx, reg, configPath, Options{ToolPrefix: "mcp"})
	if err != nil {
		t.Fatalf("register from file: %v", err)
	}
	if manager == nil {
		t.Fatal("expected manager")
	}
	defer func() { _ = manager.Close() }()

	if count != 1 {
		t.Fatalf("expected one registered tool, got %d", count)
	}
	assertMCPToolWorks(t, reg, "mcp_demo_echo")
}

func TestRegisterFromFileRegistersAndExecutesTools_StdioNDJSON(t *testing.T) {
	if os.Getenv(helperEnv) == "1" {
		runHelperServer()
		os.Exit(0)
	}

	dir := t.TempDir()
	configPath := filepath.Join(dir, "mcp-ndjson.json")
	cfg := FileConfig{
		MCPServers: map[string]ServerConfig{
			"demo": {
				Transport: "stdio",
				Framing:   "ndjson",
				Command:   os.Args[0],
				Args:      []string{"-test.run=TestRegisterFromFileRegistersAndExecutesTools_StdioNDJSON", "--"},
				Env: map[string]string{
					helperEnv:                      "1",
					"NISIX_MCP_TEST_HELPER_NDJSON": "1",
				},
			},
		},
	}
	writeMCPConfig(t, configPath, cfg)

	reg := tools.NewRegistry()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	manager, count, err := RegisterFromFile(ctx, reg, configPath, Options{ToolPrefix: "mcp"})
	if err != nil {
		t.Fatalf("register from file: %v", err)
	}
	if manager == nil {
		t.Fatal("expected manager")
	}
	defer func() { _ = manager.Close() }()

	if count != 1 {
		t.Fatalf("expected one registered tool, got %d", count)
	}
	assertMCPToolWorks(t, reg, "mcp_demo_echo")
}

func TestManagerForwardsNotificationsFromStdio(t *testing.T) {
	if os.Getenv(helperEnv) == "1" {
		runHelperServer()
		os.Exit(0)
	}

	dir := t.TempDir()
	configPath := filepath.Join(dir, "mcp-notify.json")
	cfg := FileConfig{
		MCPServers: map[string]ServerConfig{
			"demo": {
				Transport: "stdio",
				Command:   os.Args[0],
				Args:      []string{"-test.run=TestManagerForwardsNotificationsFromStdio", "--"},
				Env:       map[string]string{helperEnv: "1"},
			},
		},
	}
	writeMCPConfig(t, configPath, cfg)

	reg := tools.NewRegistry()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	manager, _, err := RegisterFromFile(ctx, reg, configPath, Options{ToolPrefix: "mcp"})
	if err != nil {
		t.Fatalf("register from file: %v", err)
	}
	if manager == nil {
		t.Fatal("expected manager")
	}
	defer func() { _ = manager.Close() }()

	if _, err := reg.Execute(ctx, "mcp_demo_echo", map[string]any{"message": "hello"}); err != nil {
		t.Fatalf("execute tool: %v", err)
	}

	note := waitForNotification(t, manager.Notifications(), 2*time.Second)
	if note.Server != "demo" {
		t.Fatalf("unexpected notification server: %#v", note)
	}
	if note.Method != "notifications/progress" {
		t.Fatalf("unexpected notification method: %#v", note)
	}
}

func TestRegisterFromFileStreamableHTTP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		msg := mustDecodeRPCRequest(t, r)
		method, _ := msg["method"].(string)
		id, _ := msg["id"]

		switch method {
		case "initialize":
			writeRPCResult(w, id, map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{},
				"serverInfo":      map[string]any{"name": "http", "version": "1.0.0"},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			writeRPCResult(w, id, map[string]any{
				"tools": []any{map[string]any{
					"name":        "echo",
					"description": "Echo test tool",
					"inputSchema": map[string]any{"type": "object"},
				}},
			})
		case "tools/call":
			params, _ := msg["params"].(map[string]any)
			args, _ := params["arguments"].(map[string]any)
			message, _ := args["message"].(string)
			if strings.EqualFold(strings.TrimSpace(message), "boom") {
				writeRPCResult(w, id, map[string]any{
					"isError": true,
					"content": []any{map[string]any{"type": "text", "text": "boom error"}},
				})
				return
			}
			writeRPCResult(w, id, map[string]any{
				"isError":           false,
				"structuredContent": map[string]any{"echo": message},
				"content":           []any{map[string]any{"type": "text", "text": "ok"}},
			})
		default:
			writeRPCError(w, id, -32601, "method not found")
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "mcp-http.json")
	cfg := FileConfig{
		MCPServers: map[string]ServerConfig{
			"http": {
				Transport: "streamable_http",
				URL:       server.URL + "/mcp",
			},
		},
	}
	writeMCPConfig(t, configPath, cfg)

	reg := tools.NewRegistry()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	manager, count, err := RegisterFromFile(ctx, reg, configPath, Options{ToolPrefix: "mcp"})
	if err != nil {
		t.Fatalf("register from file: %v", err)
	}
	if manager == nil {
		t.Fatal("expected manager")
	}
	defer func() { _ = manager.Close() }()

	if count != 1 {
		t.Fatalf("expected one registered tool, got %d", count)
	}
	assertMCPToolWorks(t, reg, "mcp_http_echo")
}

func TestRegisterFromFileTypeAliasHTTP(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		msg := mustDecodeRPCRequest(t, r)
		method, _ := msg["method"].(string)
		id, _ := msg["id"]

		switch method {
		case "initialize":
			writeRPCResult(w, id, map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{},
				"serverInfo":      map[string]any{"name": "http-alias", "version": "1.0.0"},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			writeRPCResult(w, id, map[string]any{
				"tools": []any{map[string]any{
					"name":        "echo",
					"description": "Echo test tool",
					"inputSchema": map[string]any{"type": "object"},
				}},
			})
		case "tools/call":
			params, _ := msg["params"].(map[string]any)
			args, _ := params["arguments"].(map[string]any)
			message, _ := args["message"].(string)
			if strings.EqualFold(strings.TrimSpace(message), "boom") {
				writeRPCResult(w, id, map[string]any{
					"isError": true,
					"content": []any{map[string]any{"type": "text", "text": "boom error"}},
				})
				return
			}
			writeRPCResult(w, id, map[string]any{
				"isError":           false,
				"structuredContent": map[string]any{"echo": message},
				"content":           []any{map[string]any{"type": "text", "text": "ok"}},
			})
		default:
			writeRPCError(w, id, -32601, "method not found")
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "mcp-http-type.json")
	cfg := FileConfig{
		MCPServers: map[string]ServerConfig{
			"httpalias": {
				Type: "http",
				URL:  server.URL + "/mcp",
			},
		},
	}
	writeMCPConfig(t, configPath, cfg)

	reg := tools.NewRegistry()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	manager, count, err := RegisterFromFile(ctx, reg, configPath, Options{ToolPrefix: "mcp"})
	if err != nil {
		t.Fatalf("register from file: %v", err)
	}
	if manager == nil {
		t.Fatal("expected manager")
	}
	defer func() { _ = manager.Close() }()

	if count != 1 {
		t.Fatalf("expected one registered tool, got %d", count)
	}
	assertMCPToolWorks(t, reg, "mcp_httpalias_echo")
}

func TestRegisterFromFileStreamableHTTPEventStreamResponse(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		msg := mustDecodeRPCRequest(t, r)
		method, _ := msg["method"].(string)
		id, _ := msg["id"]

		switch method {
		case "initialize":
			writeRPCResult(w, id, map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{},
				"serverInfo":      map[string]any{"name": "http-stream", "version": "1.0.0"},
			})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			writeRPCResult(w, id, map[string]any{
				"tools": []any{map[string]any{
					"name":        "echo",
					"description": "Echo test tool",
					"inputSchema": map[string]any{"type": "object"},
				}},
			})
		case "tools/call":
			params, _ := msg["params"].(map[string]any)
			args, _ := params["arguments"].(map[string]any)
			message, _ := args["message"].(string)

			w.Header().Set("Content-Type", "text/event-stream")
			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Fatalf("response writer is not flusher")
			}

			_, _ = fmt.Fprint(w, "event: message\n")
			_, _ = fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]any{
				"jsonrpc": "2.0",
				"method":  "notifications/progress",
				"params":  map[string]any{"stage": "call_started"},
			}))

			result := map[string]any{
				"isError":           false,
				"structuredContent": map[string]any{"echo": message},
				"content":           []any{map[string]any{"type": "text", "text": "ok"}},
			}
			if strings.EqualFold(strings.TrimSpace(message), "boom") {
				result = map[string]any{
					"isError": true,
					"content": []any{map[string]any{"type": "text", "text": "boom error"}},
				}
			}

			_, _ = fmt.Fprint(w, "event: message\n")
			_, _ = fmt.Fprintf(w, "data: %s\n\n", mustJSON(map[string]any{
				"jsonrpc": "2.0",
				"id":      id,
				"result":  result,
			}))
			flusher.Flush()
		default:
			writeRPCError(w, id, -32601, "method not found")
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	dir := t.TempDir()
	configPath := filepath.Join(dir, "mcp-http-stream.json")
	cfg := FileConfig{
		MCPServers: map[string]ServerConfig{
			"httpstream": {
				Transport: "streamable_http",
				URL:       server.URL + "/mcp",
			},
		},
	}
	writeMCPConfig(t, configPath, cfg)

	reg := tools.NewRegistry()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	manager, count, err := RegisterFromFile(ctx, reg, configPath, Options{ToolPrefix: "mcp"})
	if err != nil {
		t.Fatalf("register from file: %v", err)
	}
	if manager == nil {
		t.Fatal("expected manager")
	}
	defer func() { _ = manager.Close() }()

	if count != 1 {
		t.Fatalf("expected one registered tool, got %d", count)
	}
	assertMCPToolWorks(t, reg, "mcp_httpstream_echo")

	note := waitForNotification(t, manager.Notifications(), 2*time.Second)
	if note.Server != "httpstream" || note.Method != "notifications/progress" {
		t.Fatalf("unexpected notification: %#v", note)
	}
}

func TestRegisterFromFileSSE(t *testing.T) {
	h := newSSEHarness(t)
	mux := http.NewServeMux()
	mux.HandleFunc("/sse", h.handleSSE)
	mux.HandleFunc("/messages", h.handleMessages)

	server := httptest.NewServer(mux)
	defer server.Close()
	h.baseURL = server.URL

	dir := t.TempDir()
	configPath := filepath.Join(dir, "mcp-sse.json")
	cfg := FileConfig{
		MCPServers: map[string]ServerConfig{
			"sse": {
				Transport:  "sse",
				URL:        server.URL + "/sse",
				TimeoutSec: 1,
			},
		},
	}
	writeMCPConfig(t, configPath, cfg)

	reg := tools.NewRegistry()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	manager, count, err := RegisterFromFile(ctx, reg, configPath, Options{ToolPrefix: "mcp"})
	if err != nil {
		t.Fatalf("register from file: %v", err)
	}
	if manager == nil {
		t.Fatal("expected manager")
	}
	defer func() { _ = manager.Close() }()

	if count != 1 {
		t.Fatalf("expected one registered tool, got %d", count)
	}

	// Ensure SSE stream is not force-closed by request timeout.
	time.Sleep(1500 * time.Millisecond)
	assertMCPToolWorks(t, reg, "mcp_sse_echo")
}

func TestRegisterFromFileMissingConfigNoop(t *testing.T) {
	reg := tools.NewRegistry()
	manager, count, err := RegisterFromFile(context.Background(), reg, filepath.Join(t.TempDir(), "missing-mcp.json"), Options{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if manager != nil {
		t.Fatalf("expected nil manager, got %#v", manager)
	}
	if count != 0 {
		t.Fatalf("expected zero tools, got %d", count)
	}
}

func assertMCPToolWorks(t *testing.T, reg *tools.Registry, name string) {
	t.Helper()
	if _, ok := reg.Get(name); !ok {
		t.Fatalf("expected tool %q to be registered, got %#v", name, reg.List())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	res, err := reg.Execute(ctx, name, map[string]any{"message": "hello"})
	if err != nil {
		t.Fatalf("execute tool: %v", err)
	}
	out, ok := res.Data.(map[string]any)
	if !ok {
		t.Fatalf("expected map output, got %#v", res.Data)
	}
	sc, ok := out["structuredContent"].(map[string]any)
	if !ok {
		t.Fatalf("expected structuredContent in output, got %#v", out)
	}
	if sc["echo"] != "hello" {
		t.Fatalf("unexpected tool output: %#v", sc)
	}

	_, err = reg.Execute(ctx, name, map[string]any{"message": "boom"})
	if err == nil {
		t.Fatal("expected error from MCP tool when helper returns isError=true")
	}
}

func waitForNotification(t *testing.T, notes <-chan Notification, timeout time.Duration) Notification {
	t.Helper()
	select {
	case note := <-notes:
		return note
	case <-time.After(timeout):
		t.Fatal("timeout waiting for MCP notification")
		return Notification{}
	}
}

func writeMCPConfig(t *testing.T, path string, cfg FileConfig) {
	t.Helper()
	raw, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func mustDecodeRPCRequest(t *testing.T, r *http.Request) map[string]any {
	t.Helper()
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		t.Fatalf("read request body: %v", err)
	}
	var msg map[string]any
	if err := json.Unmarshal(body, &msg); err != nil {
		t.Fatalf("decode request body: %v (body=%s)", err, string(body))
	}
	return msg
}

func writeRPCResult(w http.ResponseWriter, id any, result any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	})
}

func writeRPCError(w http.ResponseWriter, id any, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}

type sseHarness struct {
	t       *testing.T
	baseURL string

	mu      sync.Mutex
	writer  io.Writer
	flusher http.Flusher
}

func newSSEHarness(t *testing.T) *sseHarness {
	return &sseHarness{t: t}
}

func (h *sseHarness) handleSSE(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		h.t.Fatalf("response writer is not flusher")
	}

	h.mu.Lock()
	h.writer = w
	h.flusher = flusher
	h.mu.Unlock()

	h.writeSSE("endpoint", "/messages")
	<-r.Context().Done()
}

func (h *sseHarness) handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	msg := mustDecodeRPCRequest(h.t, r)
	method, _ := msg["method"].(string)
	id, hasID := msg["id"]
	if !hasID {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	var result map[string]any
	switch method {
	case "initialize":
		result = map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"serverInfo":      map[string]any{"name": "sse", "version": "1.0.0"},
		}
	case "tools/list":
		result = map[string]any{
			"tools": []any{map[string]any{
				"name":        "echo",
				"description": "Echo test tool",
				"inputSchema": map[string]any{"type": "object"},
			}},
		}
	case "tools/call":
		params, _ := msg["params"].(map[string]any)
		args, _ := params["arguments"].(map[string]any)
		message, _ := args["message"].(string)
		if strings.EqualFold(strings.TrimSpace(message), "boom") {
			result = map[string]any{
				"isError": true,
				"content": []any{map[string]any{"type": "text", "text": "boom error"}},
			}
		} else {
			result = map[string]any{
				"isError":           false,
				"structuredContent": map[string]any{"echo": message},
				"content":           []any{map[string]any{"type": "text", "text": "ok"}},
			}
		}
	default:
		result = map[string]any{}
	}

	h.writeSSE("message", mustJSON(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
	}))
	w.WriteHeader(http.StatusAccepted)
}

func (h *sseHarness) writeSSE(event, data string) {
	deadline := time.Now().Add(3 * time.Second)
	for {
		h.mu.Lock()
		writer := h.writer
		flusher := h.flusher
		h.mu.Unlock()
		if writer != nil && flusher != nil {
			_, _ = fmt.Fprintf(writer, "event: %s\n", event)
			for _, line := range strings.Split(data, "\n") {
				_, _ = fmt.Fprintf(writer, "data: %s\n", line)
			}
			_, _ = fmt.Fprint(writer, "\n")
			flusher.Flush()
			return
		}
		if time.Now().After(deadline) {
			h.t.Fatalf("sse writer not ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func runHelperServer() {
	useNDJSON := os.Getenv("NISIX_MCP_TEST_HELPER_NDJSON") == "1"
	reader := bufio.NewReader(os.Stdin)
	writer := bufio.NewWriter(os.Stdout)
	for {
		var body []byte
		var err error
		if useNDJSON {
			body, err = readHelperNDJSON(reader)
		} else {
			body, err = readHelperFrame(reader)
		}
		if err != nil {
			return
		}
		var msg map[string]any
		if err := json.Unmarshal(body, &msg); err != nil {
			continue
		}
		method, _ := msg["method"].(string)
		id, hasID := msg["id"]

		write := writeHelperResponse
		if useNDJSON {
			write = writeHelperResponseNDJSON
		}

		switch method {
		case "initialize":
			if hasID {
				write(writer, map[string]any{
					"jsonrpc": "2.0",
					"id":      id,
					"result": map[string]any{
						"protocolVersion": "2024-11-05",
						"capabilities":    map[string]any{},
						"serverInfo":      map[string]any{"name": "helper", "version": "1.0.0"},
					},
				})
			}
		case "notifications/initialized":
			continue
		case "tools/list":
			if hasID {
				write(writer, map[string]any{
					"jsonrpc": "2.0",
					"id":      id,
					"result": map[string]any{
						"tools": []any{
							map[string]any{
								"name":        "echo",
								"description": "Echo test tool",
								"inputSchema": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"message": map[string]any{"type": "string"},
									},
								},
							},
						},
					},
				})
			}
		case "tools/call":
			params, _ := msg["params"].(map[string]any)
			args, _ := params["arguments"].(map[string]any)
			message, _ := args["message"].(string)
			if hasID {
				write(writer, map[string]any{
					"jsonrpc": "2.0",
					"method":  "notifications/progress",
					"params":  map[string]any{"stage": "call_started"},
				})
				if strings.EqualFold(strings.TrimSpace(message), "boom") {
					write(writer, map[string]any{
						"jsonrpc": "2.0",
						"id":      id,
						"result": map[string]any{
							"isError": true,
							"content": []any{map[string]any{"type": "text", "text": "boom error"}},
						},
					})
					continue
				}
				write(writer, map[string]any{
					"jsonrpc": "2.0",
					"id":      id,
					"result": map[string]any{
						"isError":           false,
						"structuredContent": map[string]any{"echo": message},
						"content":           []any{map[string]any{"type": "text", "text": "ok"}},
					},
				})
			}
		default:
			if hasID {
				write(writer, map[string]any{
					"jsonrpc": "2.0",
					"id":      id,
					"error": map[string]any{
						"code":    -32601,
						"message": fmt.Sprintf("method not found: %s", method),
					},
				})
			}
		}
	}
}

func writeHelperResponse(writer *bufio.Writer, payload map[string]any) {
	body, _ := json.Marshal(payload)
	var frame bytes.Buffer
	_, _ = fmt.Fprintf(&frame, "Content-Length: %d\r\n\r\n", len(body))
	frame.Write(body)
	_, _ = writer.Write(frame.Bytes())
	_ = writer.Flush()
}

func writeHelperResponseNDJSON(writer *bufio.Writer, payload map[string]any) {
	body, _ := json.Marshal(payload)
	_, _ = writer.Write(append(body, '\n'))
	_ = writer.Flush()
}

func readHelperFrame(reader *bufio.Reader) ([]byte, error) {
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
		if strings.EqualFold(strings.TrimSpace(parts[0]), "Content-Length") {
			n, err := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil {
				return nil, err
			}
			contentLength = n
		}
	}
	if contentLength < 0 {
		return nil, io.EOF
	}
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, body); err != nil {
		return nil, err
	}
	return body, nil
}

func readHelperNDJSON(reader *bufio.Reader) ([]byte, error) {
	line, err := reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	return []byte(strings.TrimSpace(line)), nil
}
