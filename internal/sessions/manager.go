package sessions

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type MessageRecord struct {
	Type       string    `json:"type"`
	Role       string    `json:"role"`
	SessionID  string    `json:"sessionId"`
	SessionKey string    `json:"sessionKey"`
	AgentID    string    `json:"agentId"`
	Text       string    `json:"text"`
	At         time.Time `json:"at"`
}

type Manager struct {
	mu            sync.Mutex
	store         Store
	transcriptDir string
}

type HistoryFilter struct {
	Limit  int
	Role   string
	From   time.Time
	To     time.Time
	Before time.Time
	After  time.Time
	Cursor string
}

type HistoryPage struct {
	Messages   []MessageRecord `json:"messages"`
	NextCursor string          `json:"nextCursor,omitempty"`
	PrevCursor string          `json:"prevCursor,omitempty"`
	Total      int             `json:"total"`
}

func NewManager(store Store, transcriptDir string) *Manager {
	return &Manager{store: store, transcriptDir: transcriptDir}
}

func (m *Manager) Touch(sessionKey, agentID string) (Entry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if existing, ok := m.store.Get(sessionKey); ok {
		existing.UpdatedAt = time.Now().UTC()
		if err := m.store.Put(existing); err != nil {
			return Entry{}, err
		}
		return existing, nil
	}

	entry := Entry{
		SessionKey: sessionKey,
		SessionID:  fmt.Sprintf("sess-%d", time.Now().UnixNano()),
		AgentID:    agentID,
		UpdatedAt:  time.Now().UTC(),
	}
	if err := m.store.Put(entry); err != nil {
		return Entry{}, err
	}
	return entry, nil
}

func (m *Manager) Append(entry Entry, role, text string) error {
	if text == "" {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	record := MessageRecord{
		Type:       "message",
		Role:       role,
		SessionID:  entry.SessionID,
		SessionKey: entry.SessionKey,
		AgentID:    entry.AgentID,
		Text:       text,
		At:         time.Now().UTC(),
	}

	if err := os.MkdirAll(m.transcriptDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(m.transcriptDir, entry.SessionID+".jsonl")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	b, err := json.Marshal(record)
	if err != nil {
		return err
	}
	if _, err := f.Write(append(b, '\n')); err != nil {
		return err
	}
	return nil
}

func (m *Manager) List() []Entry {
	return m.store.List()
}

func (m *Manager) History(sessionKey string, limit int) ([]MessageRecord, error) {
	return m.HistoryFiltered(sessionKey, HistoryFilter{Limit: limit})
}

func (m *Manager) HistoryFiltered(sessionKey string, filter HistoryFilter) ([]MessageRecord, error) {
	page, err := m.HistoryPageFiltered(sessionKey, filter)
	if err != nil {
		return nil, err
	}
	return page.Messages, nil
}

func (m *Manager) HistoryPageFiltered(sessionKey string, filter HistoryFilter) (HistoryPage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entry, ok := m.store.Get(sessionKey)
	if !ok {
		return HistoryPage{}, errors.New("sessions: session key not found")
	}
	path := filepath.Join(m.transcriptDir, entry.SessionID+".jsonl")
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return HistoryPage{Messages: []MessageRecord{}}, nil
		}
		return HistoryPage{}, err
	}
	lines := bytes.Split(b, []byte{'\n'})
	out := make([]MessageRecord, 0, len(lines))
	for _, line := range lines {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var rec MessageRecord
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		if filter.Role != "" && rec.Role != filter.Role {
			continue
		}
		if !filter.From.IsZero() && rec.At.Before(filter.From) {
			continue
		}
		if !filter.To.IsZero() && rec.At.After(filter.To) {
			continue
		}
		if !filter.Before.IsZero() && !rec.At.Before(filter.Before) {
			continue
		}
		if !filter.After.IsZero() && !rec.At.After(filter.After) {
			continue
		}
		out = append(out, rec)
	}

	total := len(out)
	limit := filter.Limit
	if limit <= 0 {
		limit = 50
	}
	start := 0
	if cursor := strings.TrimSpace(filter.Cursor); cursor != "" {
		if parsed, err := strconv.Atoi(cursor); err == nil {
			start = parsed
		}
	} else if total > limit {
		start = total - limit
	}
	if start < 0 {
		start = 0
	}
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}

	page := HistoryPage{
		Messages: out[start:end],
		Total:    total,
	}
	if end < total {
		page.NextCursor = strconv.Itoa(end)
	}
	if start > 0 {
		prevStart := start - limit
		if prevStart < 0 {
			prevStart = 0
		}
		page.PrevCursor = strconv.Itoa(prevStart)
	}
	return page, nil
}
