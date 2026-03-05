package sessions

import (
	"encoding/json"
	"os"
	"path/filepath"
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
