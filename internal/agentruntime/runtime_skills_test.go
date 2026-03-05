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
	"github.com/ni6io/nisix/internal/model"
	"github.com/ni6io/nisix/internal/skills"
	"github.com/ni6io/nisix/internal/toolpolicy"
	"github.com/ni6io/nisix/internal/tools"
)

func TestRuntimeInjectsSelectedSkill(t *testing.T) {
	workspaceDir := t.TempDir()
	skillPath := filepath.Join(workspaceDir, "skills", "example", "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(skillPath), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	content := `---
name: architecture
description: architecture planning and implementation steps
---
Use concise implementation steps.`
	if err := os.WriteFile(skillPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	r := New(
		tools.NewRegistry(),
		toolpolicy.Policy{},
		memory.NewService(workspaceDir),
		domain.AgentIdentity{Name: "Assistant"},
		"",
		workspaceDir,
		nil,
		nil,
		skills.NewService(skills.Config{Enabled: true, AutoMatch: true, MaxInjected: 1}, slog.New(slog.NewTextHandler(io.Discard, nil))),
		model.NewEchoClient(),
		"dm_only",
		"hybrid",
		true,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	events := r.Run(context.Background(), domain.RunRequest{
		AgentID:    "main",
		SessionKey: "agent:main:test",
		Message: domain.InboundMessage{
			Text: "Please give architecture implementation steps",
			At:   time.Now(),
		},
	})

	final := ""
	for evt := range events {
		if evt.Done {
			final = evt.Text
		}
	}
	if !strings.Contains(final, "## Skill: architecture") {
		t.Fatalf("expected skill injection in final text, got: %q", final)
	}
}
