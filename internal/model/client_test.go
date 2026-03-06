package model

import (
	"strings"
	"testing"

	"github.com/ni6io/nisix/internal/domain"
)

func TestBuildSystemPromptOrder(t *testing.T) {
	req := Request{
		Identity: domain.AgentIdentity{
			Name:   "Nisix",
			Avatar: "N",
			Emoji:  "compass",
		},
		SoulText:            "Be concise.",
		ProjectContext:      "# Project Context\n\n## AGENTS.md\nagents\n\n## TOOLS.md\ntools\n\n## USER.md\nuser",
		SkillPrompt:         "## Skill: architecture\nUse phased rollout.",
		ConversationSummary: "Earlier the user introduced themself.",
		MemoryHits:          []string{"/tmp/memory/a.md"},
	}

	prompt := BuildSystemPrompt(req)
	idxIdentity := strings.Index(prompt, "You are Nisix.")
	idxSoul := strings.Index(prompt, "SOUL instructions:")
	idxAgents := strings.Index(prompt, "## AGENTS.md")
	idxTools := strings.Index(prompt, "## TOOLS.md")
	idxUser := strings.Index(prompt, "## USER.md")
	if idxIdentity < 0 || idxSoul < 0 || idxAgents < 0 || idxTools < 0 || idxUser < 0 {
		t.Fatalf("missing expected prompt sections: %q", prompt)
	}
	if !(idxIdentity < idxSoul && idxSoul < idxAgents && idxAgents < idxTools && idxTools < idxUser) {
		t.Fatalf("unexpected prompt order: %q", prompt)
	}
	if strings.Count(prompt, "Project context:") != 0 {
		t.Fatalf("unexpected duplicate project context heading: %q", prompt)
	}
	if !strings.Contains(prompt, "Active skills:") || !strings.Contains(prompt, "Relevant memory files:") {
		t.Fatalf("missing skill/memory sections: %q", prompt)
	}
	if !strings.Contains(prompt, "Conversation summary:") {
		t.Fatalf("missing conversation summary section: %q", prompt)
	}
}

func TestBuildUserPromptIncludesConversationHistory(t *testing.T) {
	req := Request{
		UserText: "What is my name?",
		History: []domain.ConversationMessage{
			{Role: "user", Text: "My name is Thanh"},
			{Role: "assistant", Text: "Noted."},
		},
	}

	prompt := BuildUserPrompt(req)
	if !strings.Contains(prompt, "Conversation history:") {
		t.Fatalf("expected conversation history heading, got %q", prompt)
	}
	if !strings.Contains(prompt, "User: My name is Thanh") {
		t.Fatalf("expected user history entry, got %q", prompt)
	}
	if !strings.Contains(prompt, "Assistant: Noted.") {
		t.Fatalf("expected assistant history entry, got %q", prompt)
	}
	if !strings.HasSuffix(prompt, "What is my name?") {
		t.Fatalf("expected current user message at end, got %q", prompt)
	}
}
