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
	"github.com/ni6io/nisix/internal/profile"
	"github.com/ni6io/nisix/internal/skills"
	"github.com/ni6io/nisix/internal/toolpolicy"
	"github.com/ni6io/nisix/internal/tools"
)

func TestRuntimeProfileListAndDiffCommands(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "USER.md"), []byte("# USER\n\n## Profile\n- **Name:** Old\n"), 0o644); err != nil {
		t.Fatalf("seed user: %v", err)
	}

	profileSvc := profile.NewService(workspace, profile.Config{
		UpdateMode:        "hybrid",
		AutoDetectEnabled: true,
		AllowedFiles:      []string{"USER.md", "IDENTITY.md"},
		MaxFileBytes:      1024,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	r := New(
		tools.NewRegistry(),
		toolpolicy.Policy{},
		memory.NewService(workspace),
		domain.AgentIdentity{Name: "Assistant"},
		"",
		workspace,
		nil,
		profileSvc,
		skills.NewService(skills.Config{Enabled: true, AutoMatch: true, MaxInjected: 1}, slog.New(slog.NewTextHandler(io.Discard, nil))),
		model.NewEchoClient(),
		"dm_only",
		"hybrid",
		true,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)

	final := runAndFinalText(t, r, "/profile list")
	if !strings.Contains(final, "allowed profile files:") || !strings.Contains(final, "USER.md") {
		t.Fatalf("unexpected profile list output: %q", final)
	}

	final = runAndFinalText(t, r, "/profile diff USER.md\n# USER\n\n## Profile\n- **Name:** New")
	if !strings.Contains(final, "## Diff USER.md") || !strings.Contains(final, "+ - **Name:** New") {
		t.Fatalf("unexpected profile diff output: %q", final)
	}
}

func runAndFinalText(t *testing.T, r *Runtime, text string) string {
	t.Helper()
	events := r.Run(context.Background(), domain.RunRequest{
		AgentID:    "main",
		SessionKey: "agent:main:test",
		Message: domain.InboundMessage{
			Text: text,
			At:   time.Now(),
		},
	})
	final := ""
	for evt := range events {
		if evt.Done {
			final = evt.Text
		}
	}
	return final
}
