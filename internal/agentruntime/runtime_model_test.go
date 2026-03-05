package agentruntime

import (
	"context"
	"io"
	"log/slog"
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
	reply string
	calls int
}

func (m *fakeModel) Generate(_ context.Context, _ model.Request) (string, error) {
	m.calls++
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
	if final != "codex says hi" {
		t.Fatalf("unexpected final output: %q", final)
	}
}
