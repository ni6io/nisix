package model

import (
	"context"
	"fmt"
	"strings"

	"github.com/ni6io/nisix/internal/domain"
)

type Request struct {
	AgentID        string
	SessionKey     string
	UserText       string
	Identity       domain.AgentIdentity
	SoulText       string
	ProjectContext string
	SkillPrompt    string
	MemoryHits     []string
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
	if len(req.MemoryHits) > 0 {
		lines = append(lines, "Relevant memory files:")
		for _, hit := range req.MemoryHits {
			lines = append(lines, "- "+hit)
		}
	}
	return strings.Join(lines, "\n")
}
