package model

import (
	"context"
	"fmt"
	"strings"
)

type EchoClient struct{}

func NewEchoClient() *EchoClient {
	return &EchoClient{}
}

func (c *EchoClient) Generate(_ context.Context, req Request) (string, error) {
	prefix := strings.TrimSpace(req.Identity.Name)
	if prefix == "" {
		prefix = "Assistant"
	}
	reply := fmt.Sprintf("%s: %s", prefix, strings.TrimSpace(req.UserText))
	if len(req.MemoryHits) > 0 {
		reply += fmt.Sprintf(" (memory hits: %d)", len(req.MemoryHits))
	}
	if strings.TrimSpace(req.SoulText) != "" {
		reply += " [soul loaded]"
	}
	if strings.TrimSpace(req.SkillPrompt) != "" {
		reply += "\n\n" + req.SkillPrompt
	}
	if strings.TrimSpace(req.ConversationSummary) != "" {
		reply += "\n\nConversation summary:\n" + strings.TrimSpace(req.ConversationSummary)
	}
	if strings.TrimSpace(req.ProjectContext) != "" {
		reply += "\n\n" + req.ProjectContext
	}
	return reply, nil
}
