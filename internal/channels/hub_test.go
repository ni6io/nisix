package channels

import (
	"context"
	"testing"

	"github.com/ni6io/nisix/internal/domain"
)

type testSender struct {
	count int
	last  domain.OutboundMessage
}

func (s *testSender) Send(_ context.Context, msg domain.OutboundMessage) error {
	s.count++
	s.last = msg
	return nil
}

func TestMultiHubRoutesByChannelAndAccount(t *testing.T) {
	fallback := &testSender{}
	channelSender := &testSender{}
	accountSender := &testSender{}

	hub := NewMultiHub(fallback)
	hub.Register("telegram", channelSender)
	hub.RegisterAccount("telegram", "work", accountSender)

	err := hub.Send(context.Background(), domain.OutboundMessage{
		Channel:   "telegram",
		AccountID: "WORK",
		TargetID:  "1",
		Text:      "hello",
	})
	if err != nil {
		t.Fatalf("send should succeed: %v", err)
	}
	if accountSender.count != 1 {
		t.Fatalf("expected account sender count 1, got %d", accountSender.count)
	}
	if channelSender.count != 0 {
		t.Fatalf("expected channel sender untouched, got %d", channelSender.count)
	}
	if fallback.count != 0 {
		t.Fatalf("expected fallback sender untouched, got %d", fallback.count)
	}

	err = hub.Send(context.Background(), domain.OutboundMessage{
		Channel:   "telegram",
		AccountID: "unknown",
		TargetID:  "1",
		Text:      "fallback-to-channel",
	})
	if err != nil {
		t.Fatalf("send should succeed: %v", err)
	}
	if channelSender.count != 1 {
		t.Fatalf("expected channel sender count 1, got %d", channelSender.count)
	}
	if fallback.count != 0 {
		t.Fatalf("expected fallback sender untouched, got %d", fallback.count)
	}
}

func TestMultiHubUsesDefaultAccountWhenEmpty(t *testing.T) {
	fallback := &testSender{}
	defaultAccountSender := &testSender{}

	hub := NewMultiHub(fallback)
	hub.RegisterAccount("telegram", "default", defaultAccountSender)

	err := hub.Send(context.Background(), domain.OutboundMessage{
		Channel:  "telegram",
		TargetID: "1",
		Text:     "hello",
	})
	if err != nil {
		t.Fatalf("send should succeed: %v", err)
	}
	if defaultAccountSender.count != 1 {
		t.Fatalf("expected default account sender count 1, got %d", defaultAccountSender.count)
	}
	if fallback.count != 0 {
		t.Fatalf("expected fallback sender untouched, got %d", fallback.count)
	}
}
