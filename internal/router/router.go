package router

import (
	"fmt"
	"strings"

	"github.com/ni6io/nisix/internal/config"
	"github.com/ni6io/nisix/internal/domain"
)

type Resolver struct {
	cfg config.Config
}

func NewResolver(cfg config.Config) *Resolver {
	return &Resolver{cfg: cfg}
}

func (r *Resolver) Resolve(msg domain.InboundMessage) domain.Route {
	channel := strings.ToLower(strings.TrimSpace(msg.Channel))
	accountID := strings.ToLower(strings.TrimSpace(msg.AccountID))
	peerID := strings.TrimSpace(msg.PeerID)

	for _, b := range r.cfg.Bindings {
		if !matchChannel(b, channel) {
			continue
		}
		if !matchAccount(b, accountID) {
			continue
		}
		if b.Match.PeerID != "" && b.Match.PeerID != peerID {
			continue
		}
		return newRoute(b.AgentID, channel, accountID, peerID, "binding")
	}

	return newRoute(r.cfg.Agents.DefaultID, channel, accountID, peerID, "default")
}

func newRoute(agentID, channel, accountID, peerID, matchedBy string) domain.Route {
	if accountID == "" {
		accountID = "default"
	}
	if peerID == "" {
		peerID = "unknown"
	}
	return domain.Route{
		AgentID:    agentID,
		Channel:    channel,
		AccountID:  accountID,
		MatchedBy:  matchedBy,
		SessionKey: fmt.Sprintf("agent:%s:%s:%s:dm:%s", agentID, channel, accountID, peerID),
	}
}

func matchChannel(b config.BindingRule, channel string) bool {
	return strings.EqualFold(strings.TrimSpace(b.Match.Channel), channel)
}

func matchAccount(b config.BindingRule, accountID string) bool {
	rule := strings.TrimSpace(b.Match.AccountID)
	if rule == "" {
		return accountID == "" || accountID == "default"
	}
	if rule == "*" {
		return true
	}
	return strings.EqualFold(rule, accountID)
}
