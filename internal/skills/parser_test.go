package skills

import "testing"

func TestParseSkillMarkdownValid(t *testing.T) {
	meta, body, err := parseSkillMarkdown(`---
name: test-skill
description: Test description
---
Use this skill.`)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if meta.Name != "test-skill" || meta.Description != "Test description" {
		t.Fatalf("unexpected meta: %+v", meta)
	}
	if body != "Use this skill." {
		t.Fatalf("unexpected body: %q", body)
	}
}

func TestParseSkillMarkdownMissingFields(t *testing.T) {
	_, _, err := parseSkillMarkdown(`---
name: test-skill
---
Body`)
	if err == nil {
		t.Fatal("expected error for missing description")
	}
}
