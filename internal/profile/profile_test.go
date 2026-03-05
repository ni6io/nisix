package profile

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseCommand(t *testing.T) {
	cmd, ok := ParseCommand("/profile show IDENTITY.md")
	if !ok || cmd.Kind != CommandProfileShow || cmd.File != "IDENTITY.md" {
		t.Fatalf("unexpected command parse: %#v ok=%v", cmd, ok)
	}
	cmd, ok = ParseCommand("/profile list")
	if !ok || cmd.Kind != CommandProfileList {
		t.Fatalf("unexpected profile list parse: %#v ok=%v", cmd, ok)
	}
	cmd, ok = ParseCommand("/profile diff USER.md\n# USER\n\n## Profile\n- **Name:** New")
	if !ok || cmd.Kind != CommandProfileDiff || cmd.File != "USER.md" || !strings.Contains(cmd.Content, "Name:** New") {
		t.Fatalf("unexpected profile diff parse: %#v ok=%v", cmd, ok)
	}
	cmd, ok = ParseCommand("/onboard done")
	if !ok || cmd.Kind != CommandOnboardDone {
		t.Fatalf("unexpected onboard parse: %#v ok=%v", cmd, ok)
	}
}

func TestProfileUpdateReplaceAndAppend(t *testing.T) {
	ws := t.TempDir()
	svc := NewService(ws, Config{
		UpdateMode:   "hybrid",
		AllowedFiles: []string{"IDENTITY.md", "USER.md"},
		MaxFileBytes: 1024,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	res, err := svc.Update(UpdateRequest{
		File:    "IDENTITY.md",
		Mode:    UpdateModeReplace,
		Content: "name: Nisix\n",
	})
	if err != nil || !res.OK {
		t.Fatalf("replace failed: res=%+v err=%v", res, err)
	}
	res, err = svc.Update(UpdateRequest{
		File:    "IDENTITY.md",
		Mode:    UpdateModeAppend,
		Content: "emoji: compass",
	})
	if err != nil || !res.OK {
		t.Fatalf("append failed: res=%+v err=%v", res, err)
	}
	b, _ := os.ReadFile(filepath.Join(ws, "IDENTITY.md"))
	txt := string(b)
	if !strings.Contains(txt, "name: Nisix") || !strings.Contains(txt, "emoji: compass") {
		t.Fatalf("unexpected content after append: %q", txt)
	}
}

func TestProfilePatchIdentityAndUser(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "IDENTITY.md"), []byte("name: Assistant\n"), 0o644); err != nil {
		t.Fatalf("seed identity: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ws, "USER.md"), []byte("# USER\n\n## Profile\n- **Name:** Old\n"), 0o644); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	svc := NewService(ws, Config{
		UpdateMode:   "hybrid",
		AllowedFiles: []string{"IDENTITY.md", "USER.md"},
		MaxFileBytes: 1024,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if _, err := svc.Update(UpdateRequest{
		File:    "IDENTITY.md",
		Mode:    UpdateModePatch,
		Content: "name: Trinity",
	}); err != nil {
		t.Fatalf("patch identity failed: %v", err)
	}
	if _, err := svc.Update(UpdateRequest{
		File:    "USER.md",
		Mode:    UpdateModePatch,
		Content: "What to call them: Thanh",
	}); err != nil {
		t.Fatalf("patch user failed: %v", err)
	}
	identity, _ := os.ReadFile(filepath.Join(ws, "IDENTITY.md"))
	if !strings.Contains(string(identity), "name: Trinity") {
		t.Fatalf("identity not patched: %q", string(identity))
	}
	user, _ := os.ReadFile(filepath.Join(ws, "USER.md"))
	if !strings.Contains(string(user), "- **What to call them:** Thanh") {
		t.Fatalf("user not patched: %q", string(user))
	}
}

func TestHybridProposalCreateAndApply(t *testing.T) {
	ws := t.TempDir()
	svc := NewService(ws, Config{
		UpdateMode:        "hybrid",
		AutoDetectEnabled: true,
		AllowedFiles:      []string{"IDENTITY.md", "USER.md"},
		MaxFileBytes:      1024,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	p, ok, err := svc.MaybeCreateProposal("sess-1", "my name is Thanh")
	if err != nil || !ok {
		t.Fatalf("expected proposal create, p=%+v ok=%v err=%v", p, ok, err)
	}
	if _, err := svc.ApplyProposal("sess-2", p.ID); err == nil {
		t.Fatalf("expected invalid session apply error")
	}
	if _, err := svc.ApplyProposal("sess-1", p.ID); err != nil {
		t.Fatalf("apply proposal failed: %v", err)
	}
	got, err := svc.Get("USER.md")
	if err != nil {
		t.Fatalf("get user failed: %v", err)
	}
	if !strings.Contains(got.Content, "Thanh") {
		t.Fatalf("expected updated user content, got: %q", got.Content)
	}
}

func TestProfileValidationRejectsForbiddenFile(t *testing.T) {
	ws := t.TempDir()
	svc := NewService(ws, Config{
		UpdateMode:   "hybrid",
		AllowedFiles: []string{"IDENTITY.md"},
		MaxFileBytes: 16,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	if _, err := svc.Update(UpdateRequest{
		File:    "SOUL.md",
		Content: "x",
	}); err == nil || err.Error() != "FORBIDDEN_FILE" {
		t.Fatalf("expected FORBIDDEN_FILE, got %v", err)
	}
	if _, err := svc.Update(UpdateRequest{
		File:    "IDENTITY.md",
		Content: strings.Repeat("x", 100),
	}); err == nil || err.Error() != "FILE_TOO_LARGE" {
		t.Fatalf("expected FILE_TOO_LARGE, got %v", err)
	}
}

func TestPreviewAndLatestProposal(t *testing.T) {
	ws := t.TempDir()
	if err := os.WriteFile(filepath.Join(ws, "USER.md"), []byte("# USER\n\n## Profile\n- **Name:** Old\n"), 0o644); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	svc := NewService(ws, Config{
		UpdateMode:        "hybrid",
		AutoDetectEnabled: true,
		AllowedFiles:      []string{"USER.md"},
		MaxFileBytes:      1024,
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	p, ok, err := svc.MaybeCreateProposal("sess-1", "my name is Thanh")
	if err != nil || !ok {
		t.Fatalf("expected proposal create, p=%+v ok=%v err=%v", p, ok, err)
	}
	latest, ok := svc.LatestProposal("sess-1", "USER.md")
	if !ok || latest.ID != p.ID {
		t.Fatalf("expected latest proposal %s, got %+v", p.ID, latest)
	}
	proposed, err := svc.Preview(latest.Request)
	if err != nil {
		t.Fatalf("preview failed: %v", err)
	}
	if !strings.Contains(proposed, "Thanh") {
		t.Fatalf("expected preview to include patch content, got: %q", proposed)
	}
	diff := RenderLineDiff("# USER\n\n## Profile\n- **Name:** Old\n", proposed)
	if !strings.Contains(diff, "+ - **Name:** Thanh") {
		t.Fatalf("unexpected diff output: %q", diff)
	}
}
