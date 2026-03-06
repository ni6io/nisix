package model

import (
	"context"
	"fmt"
	"strings"

	"github.com/ni6io/nisix/internal/domain"
)

type Request struct {
	AgentID             string
	SessionKey          string
	UserText            string
	History             []domain.ConversationMessage
	ConversationSummary string
	Identity            domain.AgentIdentity
	SoulText            string
	ProjectContext      string
	SkillPrompt         string
	ToolPrompt          string
	MemoryHits          []string
}

type Client interface {
	Generate(ctx context.Context, req Request) (string, error)
}

func BuildSystemPrompt(req Request) string {
	lines := make([]string, 0, 18)
	name := strings.TrimSpace(req.Identity.Name)
	if name != "" {
		lines = append(lines, fmt.Sprintf("You are %s.", name))
	} else {
		lines = append(lines, "You are a helpful assistant.")
	}
	if v := strings.TrimSpace(req.Identity.Emoji); v != "" {
		lines = append(lines, fmt.Sprintf("Persona emoji: %s.", v))
	}
	if v := strings.TrimSpace(req.Identity.Avatar); v != "" {
		lines = append(lines, fmt.Sprintf("Persona avatar: %s.", v))
	}
	if v := strings.TrimSpace(req.SoulText); v != "" {
		lines = append(lines, "SOUL instructions:")
		lines = append(lines, v)
	}
	if v := strings.TrimSpace(req.ProjectContext); v != "" {
		if strings.HasPrefix(v, "#") {
			lines = append(lines, v)
		} else {
			lines = append(lines, "Project context:")
			lines = append(lines, v)
		}
	}
	if v := strings.TrimSpace(req.SkillPrompt); v != "" {
		lines = append(lines, "Active skills:")
		lines = append(lines, v)
	}
	if v := strings.TrimSpace(req.ToolPrompt); v != "" {
		lines = append(lines, "Available runtime tools:")
		lines = append(lines, v)
	}
	if v := strings.TrimSpace(req.ConversationSummary); v != "" {
		lines = append(lines, "Conversation summary:")
		lines = append(lines, v)
	}
	if len(req.MemoryHits) > 0 {
		lines = append(lines, "Relevant memory files:")
		for _, hit := range req.MemoryHits {
			lines = append(lines, "- "+hit)
		}
	}
	return strings.Join(lines, "\n")
}

func BuildUserPrompt(req Request) string {
	userText := strings.TrimSpace(req.UserText)
	if len(req.History) == 0 {
		return userText
	}

	lines := make([]string, 0, len(req.History)+3)
	lines = append(lines, "Conversation history:")
	for _, msg := range req.History {
		role := normalizeConversationRole(msg.Role)
		text := strings.TrimSpace(msg.Text)
		if text == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: %s", role, text))
	}
	lines = append(lines, "Current user message:")
	lines = append(lines, userText)
	return strings.Join(lines, "\n")
}

func normalizeConversationRole(role string) string {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "assistant":
		return "Assistant"
	default:
		return "User"
	}
}
