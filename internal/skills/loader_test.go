package skills

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadAllGatingAndDuplicate(t *testing.T) {
	root := t.TempDir()
	mustWriteSkill(t, root, "alpha", "---\nname: alpha\ndescription: alpha desc\n---\nalpha body")
	mustWriteSkill(t, root, "dup1", "---\nname: dup\ndescription: first\n---\nbody")
	mustWriteSkill(t, root, "dup2", "---\nname: dup\ndescription: second\n---\nbody")
	mustWriteSkill(t, root, "bad", "---\nname: bad\n---\nno desc")

	enabledFalse := false
	svc := NewService(Config{
		Enabled:     true,
		AutoMatch:   true,
		MaxInjected: 1,
		Allowlist:   []string{"alpha", "dup"},
		Entries: map[string]EntryConfig{
			"alpha": {Enabled: &enabledFalse},
		},
	}, slog.New(slog.NewTextHandler(io.Discard, nil)))

	skills, err := svc.LoadAll(root)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(skills) != 4 {
		t.Fatalf("expected 4 skills, got %d", len(skills))
	}

	assertReason(t, skills, "alpha", "entry_disabled")
	assertReason(t, skills, "bad", "parse_error")
	assertReason(t, skills, "dup", "")

	dupBlocked := 0
	for _, sk := range skills {
		if sk.Name == "dup" && !sk.Enabled && sk.Reason == "duplicate_name" {
			dupBlocked++
		}
	}
	if dupBlocked != 1 {
		t.Fatalf("expected one duplicate_name block, got %d", dupBlocked)
	}
}

func TestLoadAllGlobalDisabled(t *testing.T) {
	root := t.TempDir()
	mustWriteSkill(t, root, "alpha", "---\nname: alpha\ndescription: alpha desc\n---\nalpha body")

	svc := NewService(Config{Enabled: false}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	skills, err := svc.LoadAll(root)
	if err != nil {
		t.Fatalf("load failed: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Enabled || skills[0].Reason != "global_disabled" {
		t.Fatalf("unexpected global disabled status: %#v", skills[0])
	}
}

func mustWriteSkill(t *testing.T, workspace string, dir string, content string) {
	t.Helper()
	path := filepath.Join(workspace, "skills", dir, "SKILL.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}
}

func assertReason(t *testing.T, skills []Skill, name string, reason string) {
	t.Helper()
	for _, sk := range skills {
		if sk.Name == name && sk.Reason == reason {
			return
		}
	}
	t.Fatalf("skill %q with reason %q not found: %#v", name, reason, skills)
}
