package gateway

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/ni6io/nisix/internal/agentruntime"
	"github.com/ni6io/nisix/internal/bootstrap"
	"github.com/ni6io/nisix/internal/config"
	"github.com/ni6io/nisix/internal/domain"
	"github.com/ni6io/nisix/internal/memory"
	"github.com/ni6io/nisix/internal/model"
	"github.com/ni6io/nisix/internal/profile"
	"github.com/ni6io/nisix/internal/router"
	"github.com/ni6io/nisix/internal/security"
	"github.com/ni6io/nisix/internal/sessions"
	"github.com/ni6io/nisix/internal/skills"
	"github.com/ni6io/nisix/internal/toolpolicy"
	"github.com/ni6io/nisix/internal/tools"
	"github.com/ni6io/nisix/internal/workspace"
)

type noopHub struct{}

func (h *noopHub) Send(_ context.Context, _ domain.OutboundMessage) error {
	return nil
}

func TestWSConnectSkillsListAndChatFlows(t *testing.T) {
	workspace := t.TempDir()
	writeSkill(t, workspace, "architecture", "---\nname: architecture\ndescription: architecture planning and implementation steps\n---\nUse concise implementation steps.")
	writeSkill(t, workspace, "blocked", "---\nname: blocked\ndescription: blocked skill\n---\nBlocked body")

	enabledFalse := false
	skillSvc := skills.NewService(skills.Config{
		Enabled:      true,
		AutoMatch:    true,
		MaxInjected:  1,
		Allowlist:    []string{},
		MaxBodyChars: 2000,
		Entries: map[string]skills.EntryConfig{
			"blocked": {Enabled: &enabledFalse},
		},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	reg := tools.NewRegistry()
	reg.Register(tools.NewNowTool())

	rt := agentruntime.New(
		reg,
		toolpolicy.Policy{},
		memory.NewService(workspace),
		domain.AgentIdentity{Name: "Assistant"},
		"",
		workspace,
		nil,
		nil,
		skillSvc,
		model.NewEchoClient(),
		"dm_only",
		"hybrid",
		true,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	cfg := config.Config{
		Agents: config.AgentsConfig{
			DefaultID: "main",
			List:      []config.AgentConfig{{ID: "main", Workspace: workspace}},
		},
		Bindings: []config.BindingRule{{
			AgentID: "main",
			Match: config.BindingMatch{
				Channel:   "telegram",
				AccountID: "*",
			},
		}},
	}

	sessionManager := sessions.NewManager(sessions.NewInMemoryStore(), t.TempDir())
	server := New(
		router.NewResolver(cfg),
		rt,
		&noopHub{},
		security.NewTokenAuthenticator("test-token"),
		sessionManager,
		nil,
		nil,
		skillSvc,
		reg,
		workspace,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	mux := httptest.NewServer(server.WSHandler())
	defer mux.Close()
	wsURL := strings.Replace(mux.URL, "http://", "ws://", 1)

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer conn.Close()

	mustSendReq(t, conn, map[string]any{
		"type":   "req",
		"id":     "1",
		"method": "connect",
		"params": map[string]any{
			"minProtocol": 1,
			"maxProtocol": 1,
			"client": map[string]any{
				"id":       "test",
				"version":  "0.1.0",
				"platform": "test",
			},
			"auth": map[string]any{"token": "test-token"},
		},
	})
	frame := mustReadFrame(t, conn, 2*time.Second)
	if frame["type"] != "res" || frame["ok"] != true {
		t.Fatalf("unexpected connect response: %#v", frame)
	}
	connectPayload := frame["payload"].(map[string]any)
	features := connectPayload["features"].(map[string]any)
	methods := features["methods"].([]any)
	if !containsString(methods, "tools.catalog") {
		t.Fatalf("expected tools.catalog in methods list: %#v", methods)
	}

	mustSendReq(t, conn, map[string]any{
		"type":   "req",
		"id":     "2a",
		"method": "tools.catalog",
		"params": map[string]any{},
	})
	frame = mustReadFrame(t, conn, 2*time.Second)
	if frame["id"] != "2a" || frame["ok"] != true {
		t.Fatalf("unexpected tools.catalog response: %#v", frame)
	}
	payload := frame["payload"].(map[string]any)
	toolsList := payload["tools"].([]any)
	assertTool(t, toolsList, "time_now")

	mustSendReq(t, conn, map[string]any{
		"type":   "req",
		"id":     "2",
		"method": "skills.list",
		"params": map[string]any{"enabledOnly": false},
	})
	frame = mustReadFrame(t, conn, 2*time.Second)
	if frame["id"] != "2" || frame["ok"] != true {
		t.Fatalf("unexpected skills.list response: %#v", frame)
	}
	payload = frame["payload"].(map[string]any)
	skillsList := payload["skills"].([]any)
	if len(skillsList) < 2 {
		t.Fatalf("expected at least 2 skills, got %d", len(skillsList))
	}
	assertSkillStatus(t, skillsList, "architecture", true, "")
	assertSkillStatus(t, skillsList, "blocked", false, "entry_disabled")

	mustSendReq(t, conn, map[string]any{
		"type":   "req",
		"id":     "3",
		"method": "chat.send",
		"params": map[string]any{
			"channel":   "telegram",
			"accountId": "default",
			"peerId":    "123",
			"peerType":  "direct",
			"userId":    "123",
			"text":      "/skill architecture",
		},
	})
	agentText := readUntilAgentDoneAndGetText(t, conn, "3", 3*time.Second)
	if !strings.Contains(agentText, "## Skill: architecture") {
		t.Fatalf("expected explicit skill injection, got: %q", agentText)
	}

	mustSendReq(t, conn, map[string]any{
		"type":   "req",
		"id":     "4",
		"method": "chat.send",
		"params": map[string]any{
			"channel":   "telegram",
			"accountId": "default",
			"peerId":    "123",
			"peerType":  "direct",
			"userId":    "123",
			"text":      "/skill blocked",
		},
	})
	agentText = readUntilAgentDoneAndGetText(t, conn, "4", 3*time.Second)
	if !strings.Contains(agentText, "skill request rejected") {
		t.Fatalf("expected blocked skill rejection, got: %q", agentText)
	}

	mustSendReq(t, conn, map[string]any{
		"type":   "req",
		"id":     "5",
		"method": "chat.send",
		"params": map[string]any{
			"channel":   "telegram",
			"accountId": "default",
			"peerId":    "123",
			"peerType":  "direct",
			"userId":    "123",
			"text":      "please give architecture implementation steps",
		},
	})
	agentText = readUntilAgentDoneAndGetText(t, conn, "5", 3*time.Second)
	if !strings.Contains(agentText, "## Skill: architecture") {
		t.Fatalf("expected auto-matched skill injection, got: %q", agentText)
	}

	mustSendReq(t, conn, map[string]any{
		"type":   "req",
		"id":     "6",
		"method": "chat.history",
		"params": map[string]any{
			"sessionKey": "agent:main:telegram:default:dm:123",
			"limit":      50,
		},
	})
	frame = mustReadFrame(t, conn, 2*time.Second)
	if frame["id"] != "6" || frame["ok"] != true {
		t.Fatalf("unexpected chat.history response: %#v", frame)
	}
	payload = frame["payload"].(map[string]any)
	messages := payload["messages"].([]any)
	if len(messages) == 0 {
		t.Fatalf("expected non-empty history")
	}
}

func TestWSChatHistoryFilters(t *testing.T) {
	workspace := t.TempDir()
	writeSkill(t, workspace, "architecture", "---\nname: architecture\ndescription: architecture planning\n---\nUse concise implementation steps.")

	skillSvc := skills.NewService(skills.Config{Enabled: true, AutoMatch: true, MaxInjected: 1}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	rt := agentruntime.New(
		tools.NewRegistry(),
		toolpolicy.Policy{},
		memory.NewService(workspace),
		domain.AgentIdentity{Name: "Assistant"},
		"",
		workspace,
		nil,
		nil,
		skillSvc,
		model.NewEchoClient(),
		"dm_only",
		"hybrid",
		true,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	cfg := config.Config{Agents: config.AgentsConfig{DefaultID: "main", List: []config.AgentConfig{{ID: "main", Workspace: workspace}}}, Bindings: []config.BindingRule{{AgentID: "main", Match: config.BindingMatch{Channel: "telegram", AccountID: "*"}}}}
	sessionManager := sessions.NewManager(sessions.NewInMemoryStore(), t.TempDir())
	server := New(router.NewResolver(cfg), rt, &noopHub{}, security.NewTokenAuthenticator("test-token"), sessionManager, nil, nil, skillSvc, nil, workspace, slog.New(slog.NewTextHandler(io.Discard, nil)))

	ts := httptest.NewServer(server.WSHandler())
	defer ts.Close()
	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer conn.Close()

	mustSendReq(t, conn, map[string]any{"type": "req", "id": "1", "method": "connect", "params": map[string]any{"minProtocol": 1, "maxProtocol": 1, "client": map[string]any{"id": "test", "version": "0.1.0", "platform": "test"}, "auth": map[string]any{"token": "test-token"}}})
	_ = mustReadFrame(t, conn, 2*time.Second)

	mustSendReq(t, conn, map[string]any{"type": "req", "id": "2", "method": "chat.send", "params": map[string]any{"channel": "telegram", "accountId": "default", "peerId": "999", "peerType": "direct", "userId": "999", "text": "hello world"}})
	_ = readUntilAgentDoneAndGetText(t, conn, "2", 3*time.Second)

	mustSendReq(t, conn, map[string]any{"type": "req", "id": "3", "method": "chat.history", "params": map[string]any{"sessionKey": "agent:main:telegram:default:dm:999", "limit": 50, "role": "assistant"}})
	frame := mustReadFrame(t, conn, 2*time.Second)
	payload := frame["payload"].(map[string]any)
	messages := payload["messages"].([]any)
	if len(messages) == 0 {
		t.Fatalf("expected assistant messages")
	}
	for _, m := range messages {
		msg := m.(map[string]any)
		if msg["role"] != "assistant" {
			t.Fatalf("expected assistant role only, got: %#v", msg)
		}
	}
}

func TestWSChatAbort(t *testing.T) {
	workspace := t.TempDir()
	writeSkill(t, workspace, "architecture", "---\nname: architecture\ndescription: architecture planning\n---\nUse concise implementation steps.")

	skillSvc := skills.NewService(skills.Config{Enabled: true, AutoMatch: true, MaxInjected: 1}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	rt := agentruntime.New(
		tools.NewRegistry(),
		toolpolicy.Policy{},
		memory.NewService(workspace),
		domain.AgentIdentity{Name: "Assistant"},
		"",
		workspace,
		nil,
		nil,
		skillSvc,
		model.NewEchoClient(),
		"dm_only",
		"hybrid",
		true,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	cfg := config.Config{Agents: config.AgentsConfig{DefaultID: "main", List: []config.AgentConfig{{ID: "main", Workspace: workspace}}}, Bindings: []config.BindingRule{{AgentID: "main", Match: config.BindingMatch{Channel: "telegram", AccountID: "*"}}}}
	sessionManager := sessions.NewManager(sessions.NewInMemoryStore(), t.TempDir())
	server := New(router.NewResolver(cfg), rt, &noopHub{}, security.NewTokenAuthenticator("test-token"), sessionManager, nil, nil, skillSvc, nil, workspace, slog.New(slog.NewTextHandler(io.Discard, nil)))

	ts := httptest.NewServer(server.WSHandler())
	defer ts.Close()
	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer conn.Close()

	mustSendReq(t, conn, map[string]any{"type": "req", "id": "1", "method": "connect", "params": map[string]any{"minProtocol": 1, "maxProtocol": 1, "client": map[string]any{"id": "test", "version": "0.1.0", "platform": "test"}, "auth": map[string]any{"token": "test-token"}}})
	_ = mustReadFrame(t, conn, 2*time.Second)

	mustSendReq(t, conn, map[string]any{
		"type":   "req",
		"id":     "2",
		"method": "chat.send",
		"params": map[string]any{
			"channel":   "telegram",
			"accountId": "default",
			"peerId":    "777",
			"peerType":  "direct",
			"userId":    "777",
			"text":      "!slow test abort",
		},
	})

	runID := ""
	for i := 0; i < 5; i++ {
		frame := mustReadFrame(t, conn, 2*time.Second)
		if frame["type"] != "res" || frame["id"] != "2" {
			continue
		}
		payload := frame["payload"].(map[string]any)
		runID, _ = payload["runId"].(string)
		break
	}
	if runID == "" {
		t.Fatal("expected runId from chat.send response")
	}

	mustSendReq(t, conn, map[string]any{
		"type":   "req",
		"id":     "3",
		"method": "chat.abort",
		"params": map[string]any{
			"runId": runID,
		},
	})

	abortAck := mustReadFrame(t, conn, 2*time.Second)
	if abortAck["type"] != "res" || abortAck["id"] != "3" || abortAck["ok"] != true {
		t.Fatalf("unexpected abort response: %#v", abortAck)
	}

	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		frame := mustReadFrame(t, conn, deadline.Sub(time.Now()))
		if frame["type"] != "event" || frame["event"] != "agent" {
			continue
		}
		payload := frame["payload"].(map[string]any)
		gotRunID, _ := payload["runId"].(string)
		if gotRunID != runID {
			continue
		}
		aborted, _ := payload["aborted"].(bool)
		done, _ := payload["done"].(bool)
		if aborted && done {
			return
		}
	}
	t.Fatalf("expected aborted final event for runId=%s", runID)
}

func TestWSChatHistoryCursorPagination(t *testing.T) {
	workspace := t.TempDir()
	skillSvc := skills.NewService(skills.Config{Enabled: true, AutoMatch: true, MaxInjected: 1}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	rt := agentruntime.New(
		tools.NewRegistry(),
		toolpolicy.Policy{},
		memory.NewService(workspace),
		domain.AgentIdentity{Name: "Assistant"},
		"",
		workspace,
		nil,
		nil,
		skillSvc,
		model.NewEchoClient(),
		"dm_only",
		"hybrid",
		true,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	cfg := config.Config{Agents: config.AgentsConfig{DefaultID: "main", List: []config.AgentConfig{{ID: "main", Workspace: workspace}}}, Bindings: []config.BindingRule{{AgentID: "main", Match: config.BindingMatch{Channel: "telegram", AccountID: "*"}}}}
	sessionManager := sessions.NewManager(sessions.NewInMemoryStore(), t.TempDir())
	server := New(router.NewResolver(cfg), rt, &noopHub{}, security.NewTokenAuthenticator("test-token"), sessionManager, nil, nil, skillSvc, nil, workspace, slog.New(slog.NewTextHandler(io.Discard, nil)))

	ts := httptest.NewServer(server.WSHandler())
	defer ts.Close()
	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer conn.Close()

	mustSendReq(t, conn, map[string]any{"type": "req", "id": "1", "method": "connect", "params": map[string]any{"minProtocol": 1, "maxProtocol": 1, "client": map[string]any{"id": "test", "version": "0.1.0", "platform": "test"}, "auth": map[string]any{"token": "test-token"}}})
	_ = mustReadFrame(t, conn, 2*time.Second)

	for i := 0; i < 3; i++ {
		id := strconv.Itoa(i + 2)
		mustSendReq(t, conn, map[string]any{
			"type":   "req",
			"id":     id,
			"method": "chat.send",
			"params": map[string]any{
				"channel":   "telegram",
				"accountId": "default",
				"peerId":    "555",
				"peerType":  "direct",
				"userId":    "555",
				"text":      "hello",
			},
		})
		_ = readUntilAgentDoneAndGetText(t, conn, id, 3*time.Second)
	}

	mustSendReq(t, conn, map[string]any{
		"type":   "req",
		"id":     "5",
		"method": "chat.history",
		"params": map[string]any{
			"sessionKey": "agent:main:telegram:default:dm:555",
			"limit":      2,
		},
	})
	page1 := mustReadFrame(t, conn, 2*time.Second)
	payload := page1["payload"].(map[string]any)
	msgs := payload["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	prevCursor, _ := payload["prevCursor"].(string)
	if prevCursor == "" {
		t.Fatalf("expected prevCursor in first page")
	}

	mustSendReq(t, conn, map[string]any{
		"type":   "req",
		"id":     "6",
		"method": "chat.history",
		"params": map[string]any{
			"sessionKey": "agent:main:telegram:default:dm:555",
			"limit":      2,
			"cursor":     prevCursor,
		},
	})
	page2 := mustReadFrame(t, conn, 2*time.Second)
	payload = page2["payload"].(map[string]any)
	msgs = payload["messages"].([]any)
	if len(msgs) == 0 {
		t.Fatalf("expected older messages with cursor pagination")
	}
}

func TestWSProfileAndBootstrapFlows(t *testing.T) {
	root := t.TempDir()
	workspaceDir := filepath.Join(root, "main")
	templateDir := filepath.Join(root, "templates")
	if err := os.MkdirAll(templateDir, 0o755); err != nil {
		t.Fatalf("mkdir template dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "IDENTITY.md"), []byte("name: Assistant\n"), 0o644); err != nil {
		t.Fatalf("write template identity: %v", err)
	}
	if err := workspace.EnsureLayout(workspaceDir, workspace.Options{
		BootstrapFromTemplates: true,
		TemplateDir:            templateDir,
	}); err != nil {
		t.Fatalf("ensure layout: %v", err)
	}

	skillSvc := skills.NewService(skills.Config{Enabled: true, AutoMatch: true, MaxInjected: 1}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	bootstrapSvc := bootstrap.NewService(workspaceDir, slog.New(slog.NewTextHandler(io.Discard, nil)))
	profileSvc := profile.NewService(workspaceDir, profile.Config{
		UpdateMode:        "hybrid",
		AutoDetectEnabled: true,
		AllowedFiles:      []string{"IDENTITY.md", "USER.md", "SOUL.md", "AGENTS.md", "TOOLS.md", "MEMORY.md"},
		MaxFileBytes:      262144,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	rt := agentruntime.New(
		tools.NewRegistry(),
		toolpolicy.Policy{},
		memory.NewService(workspaceDir),
		domain.AgentIdentity{Name: "Assistant"},
		"",
		workspaceDir,
		bootstrapSvc,
		profileSvc,
		skillSvc,
		model.NewEchoClient(),
		"dm_only",
		"hybrid",
		true,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	cfg := config.Config{Agents: config.AgentsConfig{DefaultID: "main", List: []config.AgentConfig{{ID: "main", Workspace: workspaceDir}}}, Bindings: []config.BindingRule{{AgentID: "main", Match: config.BindingMatch{Channel: "telegram", AccountID: "*"}}}}
	sessionManager := sessions.NewManager(sessions.NewInMemoryStore(), t.TempDir())
	server := New(router.NewResolver(cfg), rt, &noopHub{}, security.NewTokenAuthenticator("test-token"), sessionManager, profileSvc, bootstrapSvc, skillSvc, nil, workspaceDir, slog.New(slog.NewTextHandler(io.Discard, nil)))

	ts := httptest.NewServer(server.WSHandler())
	defer ts.Close()
	wsURL := strings.Replace(ts.URL, "http://", "ws://", 1)
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	defer conn.Close()

	mustSendReq(t, conn, map[string]any{"type": "req", "id": "1", "method": "connect", "params": map[string]any{"minProtocol": 1, "maxProtocol": 1, "client": map[string]any{"id": "test", "version": "0.1.0", "platform": "test"}, "auth": map[string]any{"token": "test-token"}}})
	_ = mustReadFrame(t, conn, 2*time.Second)

	mustSendReq(t, conn, map[string]any{"type": "req", "id": "2", "method": "profile.update", "params": map[string]any{"file": "USER.md", "content": "# USER\n\n## Profile\n- **Name:** Old\n", "mode": "replace", "reason": "test"}})
	res := mustReadFrame(t, conn, 2*time.Second)
	if res["type"] != "res" || res["id"] != "2" || res["ok"] != true {
		t.Fatalf("unexpected profile.update response: %#v", res)
	}

	mustSendReq(t, conn, map[string]any{"type": "req", "id": "3", "method": "profile.get", "params": map[string]any{"file": "USER.md"}})
	res = mustReadFrame(t, conn, 2*time.Second)
	if res["type"] != "res" || res["id"] != "3" || res["ok"] != true {
		t.Fatalf("unexpected profile.get response: %#v", res)
	}
	payload := res["payload"].(map[string]any)
	content, _ := payload["content"].(string)
	if !strings.Contains(content, "Old") {
		t.Fatalf("expected profile content, got %q", content)
	}

	mustSendReq(t, conn, map[string]any{"type": "req", "id": "3b", "method": "profile.get", "params": map[string]any{"file": "NOT_ALLOWED.md"}})
	res = mustReadFrame(t, conn, 2*time.Second)
	if res["type"] != "res" || res["id"] != "3b" || res["ok"] != false {
		t.Fatalf("expected profile.get forbidden response: %#v", res)
	}
	if resErr, _ := res["error"].(map[string]any); resErr["code"] != "FORBIDDEN_FILE" {
		t.Fatalf("expected FORBIDDEN_FILE code, got: %#v", res)
	}

	mustSendReq(t, conn, map[string]any{"type": "req", "id": "4", "method": "bootstrap.status", "params": map[string]any{}})
	res = mustReadFrame(t, conn, 2*time.Second)
	if res["type"] != "res" || res["id"] != "4" || res["ok"] != true {
		t.Fatalf("unexpected bootstrap.status response: %#v", res)
	}
	payload = res["payload"].(map[string]any)
	if payload["seeded"] != true {
		t.Fatalf("expected seeded=true, got %#v", payload)
	}

	mustSendReq(t, conn, map[string]any{"type": "req", "id": "5", "method": "chat.send", "params": map[string]any{"channel": "telegram", "accountId": "default", "peerId": "222", "peerType": "direct", "userId": "222", "text": "my name is Thanh"}})
	final := readUntilAgentDoneAndGetText(t, conn, "5", 3*time.Second)
	if !strings.Contains(final, "Proposal ID:") {
		t.Fatalf("expected proposal response, got: %q", final)
	}
	re := regexp.MustCompile(`Proposal ID:\s*(prop-\d+)`)
	matches := re.FindStringSubmatch(final)
	if len(matches) != 2 {
		t.Fatalf("expected proposal id in response, got: %q", final)
	}
	proposalID := matches[1]

	mustSendReq(t, conn, map[string]any{"type": "req", "id": "6", "method": "chat.send", "params": map[string]any{"channel": "telegram", "accountId": "default", "peerId": "222", "peerType": "direct", "userId": "222", "text": "/profile apply " + proposalID}})
	final = readUntilAgentDoneAndGetText(t, conn, "6", 3*time.Second)
	if !strings.Contains(final, "proposal applied") {
		t.Fatalf("expected proposal applied response, got: %q", final)
	}

	mustSendReq(t, conn, map[string]any{"type": "req", "id": "7", "method": "chat.send", "params": map[string]any{"channel": "telegram", "accountId": "default", "peerId": "222", "peerType": "direct", "userId": "222", "text": "/profile show USER.md"}})
	final = readUntilAgentDoneAndGetText(t, conn, "7", 3*time.Second)
	if !strings.Contains(final, "Thanh") {
		t.Fatalf("expected updated user profile content, got: %q", final)
	}

	mustSendReq(t, conn, map[string]any{"type": "req", "id": "8", "method": "bootstrap.complete", "params": map[string]any{"removeBootstrap": true}})
	res = mustReadFrame(t, conn, 2*time.Second)
	if res["type"] != "res" || res["id"] != "8" || res["ok"] != true {
		t.Fatalf("unexpected bootstrap.complete response: %#v", res)
	}
	payload = res["payload"].(map[string]any)
	if payload["onboardingCompleted"] != true {
		t.Fatalf("expected onboardingCompleted=true, got %#v", payload)
	}
}

func writeSkill(t *testing.T, workspace string, name string, content string) {
	t.Helper()
	path := filepath.Join(workspace, "skills", name, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
}

func mustSendReq(t *testing.T, conn *websocket.Conn, frame map[string]any) {
	t.Helper()
	if err := conn.WriteJSON(frame); err != nil {
		t.Fatalf("write json: %v", err)
	}
}

func mustReadFrame(t *testing.T, conn *websocket.Conn, timeout time.Duration) map[string]any {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	var frame map[string]any
	if err := conn.ReadJSON(&frame); err != nil {
		t.Fatalf("read json: %v", err)
	}
	return frame
}

func readUntilAgentDoneAndGetText(t *testing.T, conn *websocket.Conn, reqID string, timeout time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(timeout)
	gotAck := false
	gotDone := false
	finalText := ""

	for time.Now().Before(deadline) {
		frame := mustReadFrame(t, conn, deadline.Sub(time.Now()))
		typ, _ := frame["type"].(string)
		switch typ {
		case "res":
			id, _ := frame["id"].(string)
			if id == reqID {
				ok, _ := frame["ok"].(bool)
				if !ok {
					b, _ := json.Marshal(frame)
					t.Fatalf("request %s failed: %s", reqID, string(b))
				}
				gotAck = true
				if gotDone {
					return finalText
				}
			}
		case "event":
			event, _ := frame["event"].(string)
			if event != "agent" {
				continue
			}
			payload, _ := frame["payload"].(map[string]any)
			text, _ := payload["text"].(string)
			finalText = text
			done, _ := payload["done"].(bool)
			if done {
				gotDone = true
				if gotAck {
					return finalText
				}
			}
		}
	}
	t.Fatalf("timeout waiting for request=%s final agent event", reqID)
	return ""
}

func assertSkillStatus(t *testing.T, list []any, name string, enabled bool, reason string) {
	t.Helper()
	for _, item := range list {
		skill := item.(map[string]any)
		if skill["name"] == name {
			if skill["enabled"] != enabled {
				t.Fatalf("skill %s enabled mismatch: got %v want %v", name, skill["enabled"], enabled)
			}
			if reason != "" {
				if skill["reason"] != reason {
					t.Fatalf("skill %s reason mismatch: got %v want %v", name, skill["reason"], reason)
				}
			}
			return
		}
	}
	t.Fatalf("skill %s not found in list %#v", name, list)
}

func assertTool(t *testing.T, list []any, name string) {
	t.Helper()
	for _, item := range list {
		tool := item.(map[string]any)
		if tool["name"] != name {
			continue
		}
		inputSchema, ok := tool["inputSchema"].(map[string]any)
		if !ok {
			t.Fatalf("tool %s missing inputSchema: %#v", name, tool)
		}
		if inputSchema["type"] != "object" {
			t.Fatalf("tool %s input schema type mismatch: %#v", name, inputSchema)
		}
		return
	}
	t.Fatalf("tool %s not found in list %#v", name, list)
}

func containsString(list []any, want string) bool {
	for _, item := range list {
		if s, ok := item.(string); ok && s == want {
			return true
		}
	}
	return false
}
