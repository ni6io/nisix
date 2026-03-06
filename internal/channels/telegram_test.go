package channels

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestTelegramAllowlistModes(t *testing.T) {
	a := NewTelegramAdapter("x", TelegramOptions{
		AllowlistMode: "users_or_chats",
		AllowUsers:    []string{"1001"},
		AllowChats:    []string{"2001"},
	})
	if !a.allowedByAllowlist("2001", "9999") {
		t.Fatal("expected chat allowlist pass")
	}
	if !a.allowedByAllowlist("9999", "1001") {
		t.Fatal("expected user allowlist pass")
	}
	if a.allowedByAllowlist("9999", "9999") {
		t.Fatal("expected deny when not in allowlists")
	}

	b := NewTelegramAdapter("x", TelegramOptions{
		AllowlistMode: "users_and_chats",
		AllowUsers:    []string{"1001"},
		AllowChats:    []string{"2001"},
	})
	if !b.allowedByAllowlist("2001", "1001") {
		t.Fatal("expected users_and_chats pass")
	}
	if b.allowedByAllowlist("2001", "1002") {
		t.Fatal("expected users_and_chats deny")
	}
}

func TestTelegramStartHelpCommands(t *testing.T) {
	a := NewTelegramAdapter("x", TelegramOptions{
		BotUsername:            "nisix_bot",
		RequireMentionInGroups: true,
		EnableHelpCommands:     true,
	})
	if !a.isHelpOrStartCommand("/start") {
		t.Fatal("expected /start to match")
	}
	if !a.isHelpOrStartCommand("/help@nisix_bot details") {
		t.Fatal("expected /help@nisix_bot to match")
	}
	if a.isHelpOrStartCommand("/help@other_bot") {
		t.Fatal("expected /help@other_bot to not match this bot")
	}
}

func TestTelegramMentionPolicyAndSanitize(t *testing.T) {
	a := NewTelegramAdapter("x", TelegramOptions{
		BotUsername:            "nisix_bot",
		RequireMentionInGroups: true,
		EnableHelpCommands:     true,
	})
	if a.acceptByMentionPolicy("group", "hello there") {
		t.Fatal("expected group text without mention to be blocked")
	}
	if !a.acceptByMentionPolicy("group", "@nisix_bot hello there") {
		t.Fatal("expected mention to pass")
	}
	if a.acceptByMentionPolicy("group", "/do@other_bot task") {
		t.Fatal("expected command for another bot to be blocked")
	}
	got := a.sanitizeText("group", "@nisix_bot summarize this")
	if got != "summarize this" {
		t.Fatalf("unexpected sanitize output: %q", got)
	}

	got = a.sanitizeText("group", "/ask@nisix_bot deploy status")
	if got != "/ask deploy status" {
		t.Fatalf("unexpected command sanitize output: %q", got)
	}
}

func TestTelegramRateLimit(t *testing.T) {
	a := NewTelegramAdapter("x", TelegramOptions{
		MinUserIntervalMs: 500,
	})
	now := time.Unix(1000, 0)
	if !a.allowByRateLimit("u1", now) {
		t.Fatal("expected first message allowed")
	}
	if a.allowByRateLimit("u1", now.Add(100*time.Millisecond)) {
		t.Fatal("expected second message blocked by rate limit")
	}
	if !a.allowByRateLimit("u1", now.Add(600*time.Millisecond)) {
		t.Fatal("expected message allowed after interval")
	}
}

func TestTelegramUpdateDedupe(t *testing.T) {
	a := NewTelegramAdapter("x", TelegramOptions{
		DedupeWindow: 2,
	})
	if a.isDuplicateUpdate(100) {
		t.Fatal("expected first update not duplicate")
	}
	if !a.isDuplicateUpdate(100) {
		t.Fatal("expected duplicate update")
	}
	if a.isDuplicateUpdate(101) {
		t.Fatal("expected new update not duplicate")
	}
	if a.isDuplicateUpdate(102) {
		t.Fatal("expected new update not duplicate")
	}
	// 100 should be evicted because dedupe window is 2.
	if a.isDuplicateUpdate(100) {
		t.Fatal("expected evicted update id not duplicate")
	}
}

func TestTelegramAccountIDOption(t *testing.T) {
	a := NewTelegramAdapter("x")
	if a.accountID != "default" {
		t.Fatalf("expected default account id, got %q", a.accountID)
	}

	b := NewTelegramAdapter("x", TelegramOptions{AccountID: "Work_Bot"})
	if b.accountID != "work_bot" {
		t.Fatalf("expected normalized account id work_bot, got %q", b.accountID)
	}
}

func TestIsTransientNetErr(t *testing.T) {
	errs := []error{
		fmt.Errorf("read: connection reset by peer"),
		fmt.Errorf("use of closed network connection"),
	}
	for _, e := range errs {
		if !isTransientNetErr(e) {
			t.Fatalf("expected transient for %v", e)
		}
	}
	if isTransientNetErr(fmt.Errorf("other error")) {
		t.Fatal("expected non-transient")
	}
}

func TestChunkTelegramText(t *testing.T) {
	long := strings.Repeat("word ", 1000) // 5000+ chars
	chunks := chunkTelegramText(long, telegramMaxMessageLen)
	if len(chunks) < 2 {
		t.Fatalf("expected multiple chunks, got %d", len(chunks))
	}
	rejoined := strings.Join(chunks, "")
	if rejoined != long {
		t.Fatal("rejoined text does not match original")
	}
	for i, c := range chunks {
		if len([]rune(c)) > telegramMaxMessageLen {
			t.Fatalf("chunk %d exceeds limit: %d", i, len([]rune(c)))
		}
	}
}
