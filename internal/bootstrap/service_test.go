package bootstrap

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ni6io/nisix/internal/domain"
	"github.com/ni6io/nisix/internal/workspace"
)

func TestLoadContextReadsProjectFiles(t *testing.T) {
	ws := filepath.Join(t.TempDir(), "main")
	templateDir := filepath.Join(filepath.Dir(ws), "templates")
	if err := os.MkdirAll(templateDir, 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "IDENTITY.md"), []byte("name: Trinity"), 0o644); err != nil {
		t.Fatalf("write template identity: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "SOUL.md"), []byte("Soul text"), 0o644); err != nil {
		t.Fatalf("write template soul: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "AGENTS.md"), []byte("agents rules"), 0o644); err != nil {
		t.Fatalf("write template agents: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "TOOLS.md"), []byte("tool policy"), 0o644); err != nil {
		t.Fatalf("write template tools: %v", err)
	}
	if err := os.WriteFile(filepath.Join(templateDir, "USER.md"), []byte("user preferences"), 0o644); err != nil {
		t.Fatalf("write template user: %v", err)
	}
	if err := workspace.EnsureLayout(ws, workspace.Options{
		BootstrapFromTemplates: true,
		TemplateDir:            templateDir,
	}); err != nil {
		t.Fatalf("ensure layout: %v", err)
	}
	svc := NewService(ws, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, err := svc.LoadContext("session", domain.InboundMessage{})
	if err != nil {
		t.Fatalf("load context: %v", err)
	}
	if ctx.Identity.Name != "Trinity" {
		t.Fatalf("expected identity name Trinity, got %q", ctx.Identity.Name)
	}
	if !strings.Contains(ctx.ProjectPrompt, "# Project Context") {
		t.Fatalf("expected project context prompt, got: %q", ctx.ProjectPrompt)
	}
	if strings.Contains(ctx.ProjectPrompt, "## SOUL.md") {
		t.Fatalf("SOUL.md should not be duplicated inside project context: %q", ctx.ProjectPrompt)
	}
	agentsIdx := strings.Index(ctx.ProjectPrompt, "## AGENTS.md")
	toolsIdx := strings.Index(ctx.ProjectPrompt, "## TOOLS.md")
	userIdx := strings.Index(ctx.ProjectPrompt, "## USER.md")
	if agentsIdx < 0 || toolsIdx < 0 || userIdx < 0 {
		t.Fatalf("missing expected section order markers: %q", ctx.ProjectPrompt)
	}
	if !(agentsIdx < toolsIdx && toolsIdx < userIdx) {
		t.Fatalf("unexpected section order, want AGENTS->TOOLS->USER, got: %q", ctx.ProjectPrompt)
	}
}

func TestStatusAndComplete(t *testing.T) {
	ws := filepath.Join(t.TempDir(), "main")
	if err := os.MkdirAll(filepath.Join(filepath.Dir(ws), "templates"), 0o755); err != nil {
		t.Fatalf("mkdir templates: %v", err)
	}
	if err := workspace.EnsureLayout(ws, workspace.Options{
		BootstrapFromTemplates: true,
		TemplateDir:            filepath.Join(filepath.Dir(ws), "templates"),
	}); err != nil {
		t.Fatalf("ensure layout: %v", err)
	}
	svc := NewService(ws, slog.New(slog.NewTextHandler(io.Discard, nil)))
	st, err := svc.Status()
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !st.Seeded {
		t.Fatalf("expected seeded status")
	}
	st, err = svc.Complete(true)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if !st.OnboardingCompleted || st.BootstrapExists {
		t.Fatalf("unexpected complete status: %+v", st)
	}
}
