package gateway

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/ni6io/nisix/internal/config"
	"github.com/ni6io/nisix/internal/domain"
	"github.com/ni6io/nisix/internal/router"
	"github.com/ni6io/nisix/internal/security"
	"github.com/ni6io/nisix/internal/sessions"
)

type captureRuntime struct {
	requests []domain.RunRequest
}

func (r *captureRuntime) Run(_ context.Context, req domain.RunRequest) <-chan domain.AgentEvent {
	r.requests = append(r.requests, req)
	out := make(chan domain.AgentEvent, 1)
	out <- domain.AgentEvent{
		Kind:       "final",
		RunID:      req.RunID,
		SessionKey: req.SessionKey,
		Text:       "ok",
		Done:       true,
	}
	close(out)
	return out
}

type captureHub struct {
	messages []domain.OutboundMessage
}

func (h *captureHub) Send(_ context.Context, msg domain.OutboundMessage) error {
	h.messages = append(h.messages, msg)
	return nil
}

func TestHandleInboundPassesRecentConversationHistoryToRuntime(t *testing.T) {
	cfg := config.Config{
		Agents: config.AgentsConfig{
			DefaultID: "main",
			List:      []config.AgentConfig{{ID: "main", Workspace: t.TempDir()}},
		},
		Bindings: []config.BindingRule{{
			AgentID: "main",
			Match: config.BindingMatch{
				Channel:   "telegram",
				AccountID: "*",
			},
		}},
	}

	store := sessions.NewInMemoryStore()
	manager := sessions.NewManager(store, t.TempDir())
	entry, err := manager.Touch("agent:main:telegram:default:dm:123", "main")
	if err != nil {
		t.Fatalf("touch session: %v", err)
	}
	if err := manager.AppendWithOptions(entry, "user", "hello", sessions.AppendOptions{EventType: "message"}); err != nil {
		t.Fatalf("append user: %v", err)
	}
	if err := manager.AppendWithOptions(entry, "assistant", "hi there", sessions.AppendOptions{EventType: "message"}); err != nil {
		t.Fatalf("append assistant: %v", err)
	}
	if err := manager.AppendWithOptions(entry, "assistant", "chunk 1", sessions.AppendOptions{EventType: "message_chunk"}); err != nil {
		t.Fatalf("append chunk: %v", err)
	}
	if err := manager.AppendWithOptions(entry, "assistant", "tool output", sessions.AppendOptions{EventType: "tool_call"}); err != nil {
		t.Fatalf("append tool event: %v", err)
	}

	rt := &captureRuntime{}
	server := New(
		router.NewResolver(cfg),
		rt,
		&noopHub{},
		security.NewTokenAuthenticator("test-token"),
		manager,
		nil,
		nil,
		nil,
		nil,
		"",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	err = server.HandleInbound(context.Background(), "test-token", domain.InboundMessage{
		Channel:   "telegram",
		AccountID: "default",
		PeerID:    "123",
		PeerType:  domain.ChatTypeDirect,
		UserID:    "123",
		Text:      "what did I just say?",
		RunID:     "run-1",
		At:        time.Now(),
	})
	if err != nil {
		t.Fatalf("handle inbound: %v", err)
	}
	if len(rt.requests) != 1 {
		t.Fatalf("expected 1 runtime request, got %d", len(rt.requests))
	}
	history := rt.requests[0].History
	if len(history) != 2 {
		t.Fatalf("expected 2 history messages, got %#v", history)
	}
	if history[0].Role != "user" || history[0].Text != "hello" {
		t.Fatalf("unexpected first history message: %#v", history[0])
	}
	if history[1].Role != "assistant" || history[1].Text != "hi there" {
		t.Fatalf("unexpected second history message: %#v", history[1])
	}
	if rt.requests[0].ConversationSummary != "" {
		t.Fatalf("expected empty conversation summary for short history, got %q", rt.requests[0].ConversationSummary)
	}
}

func TestHandleInboundSummarizesOlderTranscriptMessages(t *testing.T) {
	cfg := config.Config{
		Agents: config.AgentsConfig{
			DefaultID: "main",
			List:      []config.AgentConfig{{ID: "main", Workspace: t.TempDir()}},
		},
		Bindings: []config.BindingRule{{
			AgentID: "main",
			Match: config.BindingMatch{
				Channel:   "telegram",
				AccountID: "*",
			},
		}},
	}

	store := sessions.NewInMemoryStore()
	manager := sessions.NewManager(store, t.TempDir())
	manager.SetContextBudget(sessions.ContextBudget{HistoryLimit: 4, SummaryMaxChars: 512, SummaryLineMaxChars: 120})
	entry, err := manager.Touch("agent:main:telegram:default:dm:123", "main")
	if err != nil {
		t.Fatalf("touch session: %v", err)
	}
	for i := 0; i < 10; i++ {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		text := "message " + time.Unix(int64(i+1), 0).UTC().Format(time.RFC3339)
		if err := manager.AppendWithOptions(entry, role, text, sessions.AppendOptions{EventType: "message"}); err != nil {
			t.Fatalf("append message %d: %v", i, err)
		}
	}

	rt := &captureRuntime{}
	server := New(
		router.NewResolver(cfg),
		rt,
		&noopHub{},
		security.NewTokenAuthenticator("test-token"),
		manager,
		nil,
		nil,
		nil,
		nil,
		"",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	err = server.HandleInbound(context.Background(), "test-token", domain.InboundMessage{
		Channel:   "telegram",
		AccountID: "default",
		PeerID:    "123",
		PeerType:  domain.ChatTypeDirect,
		UserID:    "123",
		Text:      "continue",
		RunID:     "run-2",
		At:        time.Now(),
	})
	if err != nil {
		t.Fatalf("handle inbound: %v", err)
	}
	if len(rt.requests) != 1 {
		t.Fatalf("expected 1 runtime request, got %d", len(rt.requests))
	}
	req := rt.requests[0]
	if len(req.History) != 4 {
		t.Fatalf("expected 4 recent history messages, got %d", len(req.History))
	}
	if req.History[0].Text != "message 1970-01-01T00:00:07Z" {
		t.Fatalf("expected history to keep newest window, got first=%q", req.History[0].Text)
	}
	if req.ConversationSummary == "" {
		t.Fatal("expected non-empty conversation summary")
	}
	if !strings.Contains(req.ConversationSummary, "Earlier conversation covered 6 messages") {
		t.Fatalf("unexpected conversation summary: %q", req.ConversationSummary)
	}
	if !strings.Contains(req.ConversationSummary, "message 1970-01-01T00:00:01Z") {
		t.Fatalf("expected summary to mention older transcript content, got %q", req.ConversationSummary)
	}
}

func TestHandleInboundDoesNotSendRawToolEventsToChannel(t *testing.T) {
	cfg := config.Config{
		Agents: config.AgentsConfig{
			DefaultID: "main",
			List:      []config.AgentConfig{{ID: "main", Workspace: t.TempDir()}},
		},
		Bindings: []config.BindingRule{{
			AgentID: "main",
			Match: config.BindingMatch{
				Channel:   "telegram",
				AccountID: "*",
			},
		}},
	}

	hub := &captureHub{}
	server := New(
		router.NewResolver(cfg),
		runtimeFunc(func(_ context.Context, req domain.RunRequest) <-chan domain.AgentEvent {
			out := make(chan domain.AgentEvent, 2)
			out <- domain.AgentEvent{Kind: "tool", RunID: "run-1", SessionKey: req.SessionKey, Text: "tool shell result: map[...]"}
			out <- domain.AgentEvent{Kind: "final", RunID: "run-1", SessionKey: req.SessionKey, Text: "AGENTS.md\nTOOLS.md", Done: true}
			close(out)
			return out
		}),
		hub,
		security.NewTokenAuthenticator("test-token"),
		sessions.NewManager(sessions.NewInMemoryStore(), t.TempDir()),
		nil,
		nil,
		nil,
		nil,
		"",
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	err := server.HandleInbound(context.Background(), "test-token", domain.InboundMessage{
		Channel:   "telegram",
		AccountID: "default",
		PeerID:    "123",
		PeerType:  domain.ChatTypeDirect,
		UserID:    "123",
		Text:      "list files",
		RunID:     "run-1",
		At:        time.Now(),
	})
	if err != nil {
		t.Fatalf("handle inbound: %v", err)
	}
	if len(hub.messages) != 1 {
		t.Fatalf("expected only final message to be sent to channel, got %#v", hub.messages)
	}
	if hub.messages[0].Text != "AGENTS.md\nTOOLS.md" {
		t.Fatalf("unexpected outbound final text: %#v", hub.messages[0])
	}
}

type runtimeFunc func(ctx context.Context, req domain.RunRequest) <-chan domain.AgentEvent

func (f runtimeFunc) Run(ctx context.Context, req domain.RunRequest) <-chan domain.AgentEvent {
	return f(ctx, req)
}
