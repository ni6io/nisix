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
	mu            sync.RWMutex
	defaultSender Sender
	byChannel     map[string]Sender
}

func NewMultiHub(defaultSender Sender) *MultiHub {
	return &MultiHub{
		defaultSender: defaultSender,
		byChannel:     make(map[string]Sender),
	}
}

func (h *MultiHub) Register(channel string, sender Sender) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.byChannel[strings.ToLower(strings.TrimSpace(channel))] = sender
}

func (h *MultiHub) Send(ctx context.Context, msg domain.OutboundMessage) error {
	key := strings.ToLower(strings.TrimSpace(msg.Channel))
	h.mu.RLock()
	sender := h.byChannel[key]
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
