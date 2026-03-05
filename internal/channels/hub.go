package channels

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/ni6io/nisix/internal/domain"
)

type Sender interface {
	Send(ctx context.Context, msg domain.OutboundMessage) error
}

type Hub interface {
	Send(ctx context.Context, msg domain.OutboundMessage) error
}

type StdoutHub struct{}

func NewStdoutHub() *StdoutHub {
	return &StdoutHub{}
}

func (h *StdoutHub) Send(_ context.Context, msg domain.OutboundMessage) error {
	fmt.Printf("[send] channel=%s account=%s target=%s session=%s text=%q\n",
		msg.Channel, msg.AccountID, msg.TargetID, msg.SessionKey, msg.Text,
	)
	return nil
}

type MultiHub struct {
	mu               sync.RWMutex
	defaultSender    Sender
	byChannel        map[string]Sender
	byChannelAccount map[string]map[string]Sender
}

func NewMultiHub(defaultSender Sender) *MultiHub {
	return &MultiHub{
		defaultSender:    defaultSender,
		byChannel:        make(map[string]Sender),
		byChannelAccount: make(map[string]map[string]Sender),
	}
}

func (h *MultiHub) Register(channel string, sender Sender) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.byChannel[normalizeHubKey(channel)] = sender
}

func (h *MultiHub) RegisterAccount(channel, accountID string, sender Sender) {
	h.mu.Lock()
	defer h.mu.Unlock()
	channelKey := normalizeHubKey(channel)
	accountKey := normalizeHubKey(accountID)
	if accountKey == "" {
		accountKey = "default"
	}
	if _, ok := h.byChannelAccount[channelKey]; !ok {
		h.byChannelAccount[channelKey] = map[string]Sender{}
	}
	h.byChannelAccount[channelKey][accountKey] = sender
}

func (h *MultiHub) Send(ctx context.Context, msg domain.OutboundMessage) error {
	channelKey := normalizeHubKey(msg.Channel)
	accountKey := normalizeHubKey(msg.AccountID)
	if accountKey == "" {
		accountKey = "default"
	}

	h.mu.RLock()
	var sender Sender
	if byAccount := h.byChannelAccount[channelKey]; byAccount != nil {
		sender = byAccount[accountKey]
	}
	if sender == nil {
		sender = h.byChannel[channelKey]
	}
	fallback := h.defaultSender
	h.mu.RUnlock()

	if sender != nil {
		return sender.Send(ctx, msg)
	}
	if fallback != nil {
		return fallback.Send(ctx, msg)
	}
	return nil
}

func normalizeHubKey(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}
