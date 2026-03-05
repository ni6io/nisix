package agentruntime

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ni6io/nisix/internal/domain"
	"github.com/ni6io/nisix/internal/memory"
	"github.com/ni6io/nisix/internal/skills"
	"github.com/ni6io/nisix/internal/toolpolicy"
	"github.com/ni6io/nisix/internal/tools"
)

func TestRuntimeSkillsListCommand(t *testing.T) {
	workspace := t.TempDir()
	writeSkillFile(t, workspace, "architecture", "---\nname: architecture\ndescription: system design\n---\nbody")
	writeSkillFile(t, workspace, "blocked", "---\nname: blocked\ndescription: blocked skill\n---\nbody")

	disabled := false
	skillSvc := skills.NewService(skills.Config{
		Enabled:      true,
		AutoMatch:    true,
		MaxInjected:  1,
		Entries:      map[string]skills.EntryConfig{"blocked": {Enabled: &disabled}},
		MaxBodyChars: 2000,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	fm := &fakeModel{reply: "should not be called"}

	r := New(
		tools.NewRegistry(),
		toolpolicy.Policy{},
		memory.NewService(workspace),
		domain.AgentIdentity{Name: "Assistant"},
		"",
		workspace,
		nil,
		nil,
		skillSvc,
		fm,
		"dm_only",
		"hybrid",
		true,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	final := runFinalForText(t, r, "/skills list")
	if !strings.Contains(final, "skills:") ||
		!strings.Contains(final, "architecture [enabled]") ||
		!strings.Contains(final, "blocked [disabled (entry_disabled)]") {
		t.Fatalf("unexpected skills list output: %q", final)
	}
	if fm.calls != 0 {
		t.Fatalf("expected command to bypass model, got calls=%d", fm.calls)
	}
}

func TestRuntimeToolsListCommand(t *testing.T) {
	workspace := t.TempDir()
	reg := tools.NewRegistry()
	reg.Register(tools.NewNowTool())
	fm := &fakeModel{reply: "should not be called"}

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

	final := runFinalForText(t, r, "/tools list")
	if !strings.Contains(final, "tools:") || !strings.Contains(final, "time_now [blocked]") {
		t.Fatalf("unexpected tools list output: %q", final)
	}
	if fm.calls != 0 {
		t.Fatalf("expected command to bypass model, got calls=%d", fm.calls)
	}
}

func runFinalForText(t *testing.T, r *Runtime, text string) string {
	t.Helper()
	events := r.Run(context.Background(), domain.RunRequest{
		AgentID:    "main",
		SessionKey: "agent:main:test",
		Message: domain.InboundMessage{
			Text: text,
			At:   time.Now(),
		},
	})
	for evt := range events {
		if evt.Done {
			return evt.Text
		}
	}
	t.Fatal("missing final event")
	return ""
}

func writeSkillFile(t *testing.T, workspace, name, content string) {
	t.Helper()
	path := filepath.Join(workspace, "skills", name, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
}
