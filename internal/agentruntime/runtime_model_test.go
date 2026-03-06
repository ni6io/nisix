package agentruntime

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/ni6io/nisix/internal/domain"
	"github.com/ni6io/nisix/internal/memory"
	"github.com/ni6io/nisix/internal/model"
	"github.com/ni6io/nisix/internal/skills"
	"github.com/ni6io/nisix/internal/toolpolicy"
	"github.com/ni6io/nisix/internal/tools"
)

type fakeModel struct {
	reply   string
	calls   int
	lastReq model.Request
}

type fakeExecTool struct {
	name string
	data any
}

func (t fakeExecTool) Name() string {
	return t.name
}

func (t fakeExecTool) Execute(_ context.Context, _ map[string]any) (tools.Result, error) {
	return tools.Result{Data: t.data}, nil
}

func (m *fakeModel) Generate(_ context.Context, req model.Request) (string, error) {
	m.calls++
	m.lastReq = req
	return m.reply, nil
}

func TestRuntimeUsesModelClientForNormalMessages(t *testing.T) {
	workspace := t.TempDir()
	fm := &fakeModel{reply: "codex says hi"}

	r := New(
		tools.NewRegistry(),
		toolpolicy.Policy{},
		memory.NewService(workspace),
		domain.AgentIdentity{Name: "Assistant"},
		"",
		workspace,
		nil,
		nil,
		skills.NewService(skills.Config{Enabled: true, AutoMatch: true, MaxInjected: 1}, slog.New(slog.NewTextHandler(io.Discard, nil))),
		fm,
		"dm_only",
		"hybrid",
		true,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	events := r.Run(context.Background(), domain.RunRequest{
		AgentID:    "main",
		SessionKey: "agent:main:test",
		Message: domain.InboundMessage{
			Text: "hello",
			At:   time.Now(),
		},
	})

	final := ""
	for evt := range events {
		if evt.Done {
			final = evt.Text
		}
	}
	if fm.calls != 1 {
		t.Fatalf("expected one model call, got %d", fm.calls)
	}
	if fm.lastReq.UserText != "hello" {
		t.Fatalf("expected user text to reach model, got %q", fm.lastReq.UserText)
	}
	if final != "codex says hi" {
		t.Fatalf("unexpected final output: %q", final)
	}
}

func TestRuntimePassesConversationHistoryToModel(t *testing.T) {
	workspace := t.TempDir()
	fm := &fakeModel{reply: "ok"}

	r := New(
		tools.NewRegistry(),
		toolpolicy.Policy{},
		memory.NewService(workspace),
		domain.AgentIdentity{Name: "Assistant"},
		"",
		workspace,
		nil,
		nil,
		skills.NewService(skills.Config{Enabled: true, AutoMatch: true, MaxInjected: 1}, slog.New(slog.NewTextHandler(io.Discard, nil))),
		fm,
		"dm_only",
		"hybrid",
		true,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	events := r.Run(context.Background(), domain.RunRequest{
		AgentID:    "main",
		SessionKey: "agent:main:test",
		History: []domain.ConversationMessage{
			{Role: "user", Text: "My name is Thanh"},
			{Role: "assistant", Text: "Noted."},
		},
		Message: domain.InboundMessage{
			Text: "What is my name?",
			At:   time.Now(),
		},
	})

	for range events {
	}
	if fm.calls != 1 {
		t.Fatalf("expected one model call, got %d", fm.calls)
	}
	if len(fm.lastReq.History) != 2 {
		t.Fatalf("expected 2 history messages, got %#v", fm.lastReq.History)
	}
	if fm.lastReq.History[0].Text != "My name is Thanh" || fm.lastReq.History[1].Role != "assistant" {
		t.Fatalf("unexpected history passed to model: %#v", fm.lastReq.History)
	}
}

func TestRuntimePassesConversationSummaryToModel(t *testing.T) {
	workspace := t.TempDir()
	fm := &fakeModel{reply: "ok"}

	r := New(
		tools.NewRegistry(),
		toolpolicy.Policy{},
		memory.NewService(workspace),
		domain.AgentIdentity{Name: "Assistant"},
		"",
		workspace,
		nil,
		nil,
		skills.NewService(skills.Config{Enabled: true, AutoMatch: true, MaxInjected: 1}, slog.New(slog.NewTextHandler(io.Discard, nil))),
		fm,
		"dm_only",
		"hybrid",
		true,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	events := r.Run(context.Background(), domain.RunRequest{
		AgentID:             "main",
		SessionKey:          "agent:main:test",
		ConversationSummary: "Earlier the user said their name is Thanh.",
		Message: domain.InboundMessage{
			Text: "What is my name?",
			At:   time.Now(),
		},
	})

	for range events {
	}
	if fm.lastReq.ConversationSummary != "Earlier the user said their name is Thanh." {
		t.Fatalf("unexpected conversation summary: %q", fm.lastReq.ConversationSummary)
	}
}

func TestRuntimePassesAllowedToolPromptToModel(t *testing.T) {
	workspace := t.TempDir()
	fm := &fakeModel{reply: "ok"}
	reg := tools.NewRegistry()
	reg.Register(tools.NewNowTool())
	reg.Register(tools.NewShellTool(workspace))

	r := New(
		reg,
		toolpolicy.Policy{Allow: []string{"time_now"}},
		memory.NewService(workspace),
		domain.AgentIdentity{Name: "Assistant"},
		"",
		workspace,
		nil,
		nil,
		skills.NewService(skills.Config{Enabled: true, AutoMatch: true, MaxInjected: 1}, slog.New(slog.NewTextHandler(io.Discard, nil))),
		fm,
		"dm_only",
		"hybrid",
		true,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	events := r.Run(context.Background(), domain.RunRequest{
		AgentID:    "main",
		SessionKey: "agent:main:test-tools",
		Message: domain.InboundMessage{
			Text: "what can you do",
			At:   time.Now(),
		},
	})

	for range events {
	}
	if !strings.Contains(fm.lastReq.ToolPrompt, "time_now") {
		t.Fatalf("expected allowed tool in tool prompt, got %q", fm.lastReq.ToolPrompt)
	}
	if strings.Contains(fm.lastReq.ToolPrompt, "shell") {
		t.Fatalf("expected blocked tool to be omitted from tool prompt, got %q", fm.lastReq.ToolPrompt)
	}
	if !strings.Contains(fm.lastReq.ToolPrompt, "Do not claim a tool was used unless you emit the exact tool call.") {
		t.Fatalf("expected strict tool instruction, got %q", fm.lastReq.ToolPrompt)
	}
	if !strings.Contains(fm.lastReq.ToolPrompt, "Never fabricate command output") {
		t.Fatalf("expected anti-fabrication tool instruction, got %q", fm.lastReq.ToolPrompt)
	}
	if !strings.Contains(fm.lastReq.ToolPrompt, "Example: time_now()") {
		t.Fatalf("expected tool example in prompt, got %q", fm.lastReq.ToolPrompt)
	}
}

func TestRuntimeExecutesToolCallFromModelOutput(t *testing.T) {
	workspace := t.TempDir()
	fm := &fakeModel{
		reply: "I have a tool available.\n\n```text\nSERVER_TIME_NOW: time_now()\n```",
	}
	reg := tools.NewRegistry()
	reg.Register(tools.NewNowTool())

	r := New(
		reg,
		toolpolicy.Policy{},
		memory.NewService(workspace),
		domain.AgentIdentity{Name: "Assistant"},
		"",
		workspace,
		nil,
		nil,
		skills.NewService(skills.Config{Enabled: true, AutoMatch: true, MaxInjected: 1}, slog.New(slog.NewTextHandler(io.Discard, nil))),
		fm,
		"dm_only",
		"hybrid",
		true,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	events := r.Run(context.Background(), domain.RunRequest{
		AgentID:    "main",
		SessionKey: "agent:main:test",
		Message: domain.InboundMessage{
			Text: "what server time now",
			At:   time.Now(),
		},
	})

	final := ""
	toolEvent := ""
	for evt := range events {
		if evt.Kind == "tool" {
			toolEvent = evt.Text
		}
		if evt.Done {
			final = evt.Text
		}
	}
	if fm.calls != 1 {
		t.Fatalf("expected one model call, got %d", fm.calls)
	}
	if !strings.Contains(toolEvent, "tool time_now result:") {
		t.Fatalf("expected tool event, got %q", toolEvent)
	}
	if !strings.HasPrefix(final, "Server time now: ") {
		t.Fatalf("expected server-time final output, got %q", final)
	}
}

func TestRuntimeExecutesInlineBacktickedToolCallFromModelOutput(t *testing.T) {
	workspace := t.TempDir()
	fm := &fakeModel{
		reply: "Da goi `time_now()` de lay gio he thong.",
	}
	reg := tools.NewRegistry()
	reg.Register(tools.NewNowTool())

	r := New(
		reg,
		toolpolicy.Policy{},
		memory.NewService(workspace),
		domain.AgentIdentity{Name: "Assistant"},
		"",
		workspace,
		nil,
		nil,
		skills.NewService(skills.Config{Enabled: true, AutoMatch: true, MaxInjected: 1}, slog.New(slog.NewTextHandler(io.Discard, nil))),
		fm,
		"dm_only",
		"hybrid",
		true,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	events := r.Run(context.Background(), domain.RunRequest{
		AgentID:    "main",
		SessionKey: "agent:main:test-inline-tool",
		Message: domain.InboundMessage{
			Text: "what server time now",
			At:   time.Now(),
		},
	})

	final := ""
	toolEvent := ""
	for evt := range events {
		if evt.Kind == "tool" {
			toolEvent = evt.Text
		}
		if evt.Done {
			final = evt.Text
		}
	}
	if !strings.Contains(toolEvent, "tool time_now result:") {
		t.Fatalf("expected inline backticked tool call to execute, got %q", toolEvent)
	}
	if !strings.HasPrefix(final, "Server time now: ") {
		t.Fatalf("expected server-time final output, got %q", final)
	}
}

func TestRuntimeBlocksToolCallFromModelOutputByPolicy(t *testing.T) {
	workspace := t.TempDir()
	fm := &fakeModel{reply: "time_now()"}
	reg := tools.NewRegistry()
	reg.Register(tools.NewNowTool())

	r := New(
		reg,
		toolpolicy.Policy{Deny: []string{"time_now"}},
		memory.NewService(workspace),
		domain.AgentIdentity{Name: "Assistant"},
		"",
		workspace,
		nil,
		nil,
		skills.NewService(skills.Config{Enabled: true, AutoMatch: true, MaxInjected: 1}, slog.New(slog.NewTextHandler(io.Discard, nil))),
		fm,
		"dm_only",
		"hybrid",
		true,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	events := r.Run(context.Background(), domain.RunRequest{
		AgentID:    "main",
		SessionKey: "agent:main:test",
		Message: domain.InboundMessage{
			Text: "what server time now",
			At:   time.Now(),
		},
	})

	final := ""
	for evt := range events {
		if evt.Done {
			final = evt.Text
		}
	}
	if final != "tool blocked by policy" {
		t.Fatalf("expected blocked message, got %q", final)
	}
}

func TestRuntimeFormatsShellToolResultForUser(t *testing.T) {
	workspace := t.TempDir()
	fm := &fakeModel{reply: "shell({\"command\":\"ls\"})"}
	reg := tools.NewRegistry()
	reg.Register(fakeExecTool{
		name: "shell",
		data: map[string]any{
			"command":   "ls",
			"cwd":       workspace,
			"exitCode":  0,
			"stdout":    "AGENTS.md\nTOOLS.md\n",
			"stderr":    "",
			"timedOut":  false,
			"truncated": false,
		},
	})

	r := New(
		reg,
		toolpolicy.Policy{},
		memory.NewService(workspace),
		domain.AgentIdentity{Name: "Assistant"},
		"",
		workspace,
		nil,
		nil,
		skills.NewService(skills.Config{Enabled: true, AutoMatch: true, MaxInjected: 1}, slog.New(slog.NewTextHandler(io.Discard, nil))),
		fm,
		"dm_only",
		"hybrid",
		true,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	final := ""
	for evt := range r.Run(context.Background(), domain.RunRequest{
		AgentID:    "main",
		SessionKey: "agent:main:test-shell-format",
		Message: domain.InboundMessage{
			Text: "list files",
			At:   time.Now(),
		},
	}) {
		if evt.Done {
			final = evt.Text
		}
	}
	if final != "AGENTS.md\nTOOLS.md" {
		t.Fatalf("expected shell stdout only, got %q", final)
	}
}

func TestRuntimeFormatsMCPToolResultForUser(t *testing.T) {
	workspace := t.TempDir()
	fm := &fakeModel{reply: "mcp_filesystem_list_directory({\"path\":\"/tmp\"})"}
	reg := tools.NewRegistry()
	reg.Register(fakeExecTool{
		name: "mcp_filesystem_list_directory",
		data: map[string]any{
			"content": []any{
				map[string]any{
					"type": "text",
					"text": "[FILE] AGENTS.md\n[DIR] skills",
				},
			},
			"structuredContent": map[string]any{
				"content": "[FILE] AGENTS.md\n[DIR] skills",
			},
		},
	})

	r := New(
		reg,
		toolpolicy.Policy{},
		memory.NewService(workspace),
		domain.AgentIdentity{Name: "Assistant"},
		"",
		workspace,
		nil,
		nil,
		skills.NewService(skills.Config{Enabled: true, AutoMatch: true, MaxInjected: 1}, slog.New(slog.NewTextHandler(io.Discard, nil))),
		fm,
		"dm_only",
		"hybrid",
		true,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	final := ""
	for evt := range r.Run(context.Background(), domain.RunRequest{
		AgentID:    "main",
		SessionKey: "agent:main:test-mcp-format",
		Message: domain.InboundMessage{
			Text: "list files",
			At:   time.Now(),
		},
	}) {
		if evt.Done {
			final = evt.Text
		}
	}
	if final != "[FILE] AGENTS.md\n[DIR] skills" {
		t.Fatalf("expected mcp text content only, got %q", final)
	}
}
