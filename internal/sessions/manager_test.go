package sessions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHistoryPageFilteredCursorPagination(t *testing.T) {
	transcriptDir := t.TempDir()
	store := NewInMemoryStore()
	mgr := NewManager(store, transcriptDir)

	entry := Entry{
		SessionKey: "agent:main:test",
		SessionID:  "sess-1",
		AgentID:    "main",
		UpdatedAt:  time.Now().UTC(),
	}
	if err := store.Put(entry); err != nil {
		t.Fatalf("put entry: %v", err)
	}

	base := time.Unix(1700000000, 0).UTC()
	writeTranscript(t, transcriptDir, entry.SessionID, []MessageRecord{
		{Role: "user", Text: "m1", At: base.Add(1 * time.Minute)},
		{Role: "assistant", Text: "m2", At: base.Add(2 * time.Minute)},
		{Role: "user", Text: "m3", At: base.Add(3 * time.Minute)},
		{Role: "assistant", Text: "m4", At: base.Add(4 * time.Minute)},
		{Role: "user", Text: "m5", At: base.Add(5 * time.Minute)},
	})

	page, err := mgr.HistoryPageFiltered(entry.SessionKey, HistoryFilter{Limit: 2})
	if err != nil {
		t.Fatalf("history page: %v", err)
	}
	if len(page.Messages) != 2 || page.Messages[0].Text != "m4" || page.Messages[1].Text != "m5" {
		t.Fatalf("unexpected default page: %#v", page.Messages)
	}
	if page.PrevCursor != "1" || page.NextCursor != "" || page.Total != 5 {
		t.Fatalf("unexpected cursors default page: %#v", page)
	}

	page, err = mgr.HistoryPageFiltered(entry.SessionKey, HistoryFilter{Limit: 2, Cursor: "1"})
	if err != nil {
		t.Fatalf("history page cursor: %v", err)
	}
	if len(page.Messages) != 2 || page.Messages[0].Text != "m2" || page.Messages[1].Text != "m3" {
		t.Fatalf("unexpected cursor page: %#v", page.Messages)
	}
	if page.PrevCursor != "0" || page.NextCursor != "3" {
		t.Fatalf("unexpected cursor pagination: %#v", page)
	}
}

func TestModelContextMaintainsRollingSummaryState(t *testing.T) {
	transcriptDir := t.TempDir()
	store := NewInMemoryStore()
	mgr := NewManager(store, transcriptDir)
	mgr.SetContextBudget(ContextBudget{HistoryLimit: 3, SummaryMaxChars: 256, SummaryLineMaxChars: 80})

	entry, err := mgr.Touch("agent:main:ctx", "main")
	if err != nil {
		t.Fatalf("touch session: %v", err)
	}
	for i, text := range []string{"u1", "a1", "u2", "a2", "u3"} {
		role := "user"
		if i%2 == 1 {
			role = "assistant"
		}
		if err := mgr.AppendWithOptions(entry, role, text, AppendOptions{EventType: "message"}); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	history, summary, err := mgr.ModelContext(entry.SessionKey)
	if err != nil {
		t.Fatalf("model context: %v", err)
	}
	if len(history) != 3 || history[0].Text != "u2" || history[2].Text != "u3" {
		t.Fatalf("unexpected rolling history: %#v", history)
	}
	if !strings.Contains(summary, "Earlier conversation covered 2 messages") {
		t.Fatalf("unexpected summary header: %q", summary)
	}
	if !strings.Contains(summary, "User: u1") || !strings.Contains(summary, "Assistant: a1") {
		t.Fatalf("expected summarized early messages, got %q", summary)
	}

	stored, ok := store.Get(entry.SessionKey)
	if !ok {
		t.Fatal("expected stored entry")
	}
	if stored.ContextStateVersion != contextStateVersion || stored.ContextStateSig == "" {
		t.Fatalf("missing persisted context state metadata: %#v", stored)
	}
	if len(stored.RecentMessages) != 3 || stored.SummarizedMessages != 2 {
		t.Fatalf("unexpected stored context state: %#v", stored)
	}
}

func TestModelContextRebuildsLegacyStateFromTranscript(t *testing.T) {
	transcriptDir := t.TempDir()
	store := NewInMemoryStore()
	mgr := NewManager(store, transcriptDir)
	mgr.SetContextBudget(ContextBudget{HistoryLimit: 2, SummaryMaxChars: 256, SummaryLineMaxChars: 80})

	entry := Entry{
		SessionKey: "agent:main:legacy",
		SessionID:  "sess-legacy",
		AgentID:    "main",
		UpdatedAt:  time.Now().UTC(),
	}
	if err := store.Put(entry); err != nil {
		t.Fatalf("put entry: %v", err)
	}

	writeTranscript(t, transcriptDir, entry.SessionID, []MessageRecord{
		{Role: "user", Text: "hello", EventType: "message", At: time.Now().UTC()},
		{Role: "assistant", Text: "hi", EventType: "message", At: time.Now().UTC()},
		{Role: "assistant", Text: "chunk", EventType: "message_chunk", Type: "event", At: time.Now().UTC()},
		{Role: "user", Text: "need help", EventType: "message", At: time.Now().UTC()},
	})

	history, summary, err := mgr.ModelContext(entry.SessionKey)
	if err != nil {
		t.Fatalf("model context: %v", err)
	}
	if len(history) != 2 || history[0].Text != "hi" || history[1].Text != "need help" {
		t.Fatalf("unexpected rebuilt history: %#v", history)
	}
	if !strings.Contains(summary, "Earlier conversation covered 1 messages") || !strings.Contains(summary, "User: hello") {
		t.Fatalf("unexpected rebuilt summary: %q", summary)
	}

	stored, ok := store.Get(entry.SessionKey)
	if !ok {
		t.Fatal("expected stored entry after rebuild")
	}
	if stored.ContextStateVersion != contextStateVersion {
		t.Fatalf("expected rebuilt context state version, got %#v", stored)
	}
}

func TestHistoryPageFilteredBeforeAfter(t *testing.T) {
	transcriptDir := t.TempDir()
	store := NewInMemoryStore()
	mgr := NewManager(store, transcriptDir)

	entry := Entry{
		SessionKey: "agent:main:test2",
		SessionID:  "sess-2",
		AgentID:    "main",
		UpdatedAt:  time.Now().UTC(),
	}
	if err := store.Put(entry); err != nil {
		t.Fatalf("put entry: %v", err)
	}

	base := time.Unix(1700000000, 0).UTC()
	t1 := base.Add(1 * time.Minute)
	t2 := base.Add(2 * time.Minute)
	t3 := base.Add(3 * time.Minute)
	writeTranscript(t, transcriptDir, entry.SessionID, []MessageRecord{
		{Role: "user", Text: "a", At: t1},
		{Role: "assistant", Text: "b", At: t2},
		{Role: "user", Text: "c", At: t3},
	})

	page, err := mgr.HistoryPageFiltered(entry.SessionKey, HistoryFilter{
		After:  t1,
		Before: t3,
		Role:   "assistant",
		Limit:  10,
	})
	if err != nil {
		t.Fatalf("history page: %v", err)
	}
	if len(page.Messages) != 1 || page.Messages[0].Text != "b" {
		t.Fatalf("unexpected filtered messages: %#v", page.Messages)
	}
}

func TestAppendWithOptionsWritesSchemaV2(t *testing.T) {
	transcriptDir := t.TempDir()
	store := NewInMemoryStore()
	mgr := NewManager(store, transcriptDir)
	entry := Entry{
		SessionKey: "agent:main:test3",
		SessionID:  "sess-3",
		AgentID:    "main",
		UpdatedAt:  time.Now().UTC(),
	}
	if err := store.Put(entry); err != nil {
		t.Fatalf("put entry: %v", err)
	}

	if err := mgr.AppendWithOptions(entry, "assistant", "tool time_now result: ...", AppendOptions{
		EventType: "tool_call",
		RunID:     "run-1",
		Kind:      "tool",
		Provider:  "tool",
		ToolCall: &ToolCallRecord{
			Name:   "time_now",
			Status: "success",
			Output: map[string]any{"now": "2026-03-05T11:00:00Z"},
		},
		Metadata: map[string]string{"channel": "telegram"},
	}); err != nil {
		t.Fatalf("append v2: %v", err)
	}

	page, err := mgr.HistoryPageFiltered(entry.SessionKey, HistoryFilter{Limit: 10})
	if err != nil {
		t.Fatalf("history page: %v", err)
	}
	if len(page.Messages) != 1 {
		t.Fatalf("expected one message, got %d", len(page.Messages))
	}
	got := page.Messages[0]
	if got.SchemaVersion != TranscriptSchemaV2 || got.EventType != "tool_call" || got.RunID != "run-1" {
		t.Fatalf("unexpected schema v2 fields: %#v", got)
	}
	if got.ToolCall == nil || got.ToolCall.Name != "time_now" || got.ToolCall.Status != "success" {
		t.Fatalf("missing tool call details: %#v", got)
	}
}

func TestHistoryNormalizesLegacyRecords(t *testing.T) {
	transcriptDir := t.TempDir()
	store := NewInMemoryStore()
	mgr := NewManager(store, transcriptDir)
	entry := Entry{
		SessionKey: "agent:main:test4",
		SessionID:  "sess-4",
		AgentID:    "main",
		UpdatedAt:  time.Now().UTC(),
	}
	if err := store.Put(entry); err != nil {
		t.Fatalf("put entry: %v", err)
	}

	path := filepath.Join(transcriptDir, entry.SessionID+".jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir transcript dir: %v", err)
	}
	legacy := map[string]any{
		"type": "message",
		"role": "assistant",
		"text": "legacy",
		"at":   time.Now().UTC(),
	}
	b, err := json.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy: %v", err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		t.Fatalf("write legacy transcript: %v", err)
	}

	page, err := mgr.HistoryPageFiltered(entry.SessionKey, HistoryFilter{Limit: 10})
	if err != nil {
		t.Fatalf("history page: %v", err)
	}
	if len(page.Messages) != 1 {
		t.Fatalf("expected one record, got %d", len(page.Messages))
	}
	got := page.Messages[0]
	if got.SchemaVersion != 1 || got.EventType != "message" {
		t.Fatalf("legacy normalization mismatch: %#v", got)
	}
	if got.SessionID != entry.SessionID || got.SessionKey != entry.SessionKey || got.AgentID != entry.AgentID {
		t.Fatalf("legacy identity fields missing after normalization: %#v", got)
	}
	if !strings.EqualFold(got.Text, "legacy") {
		t.Fatalf("unexpected text: %#v", got)
	}
}

func writeTranscript(t *testing.T, transcriptDir, sessionID string, rows []MessageRecord) {
	t.Helper()
	path := filepath.Join(transcriptDir, sessionID+".jsonl")
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create transcript: %v", err)
	}
	defer f.Close()

	for _, row := range rows {
		row.Type = "message"
		row.SessionID = sessionID
		b, err := json.Marshal(row)
		if err != nil {
			t.Fatalf("marshal row: %v", err)
		}
		if _, err := f.Write(append(b, '\n')); err != nil {
			t.Fatalf("write row: %v", err)
		}
	}
}
