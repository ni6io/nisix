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

const TranscriptSchemaV2 = 2

type MessageRecord struct {
	SchemaVersion int               `json:"schemaVersion,omitempty"`
	Type          string            `json:"type"`
	EventType     string            `json:"eventType,omitempty"`
	Role          string            `json:"role"`
	SessionID     string            `json:"sessionId"`
	SessionKey    string            `json:"sessionKey"`
	AgentID       string            `json:"agentId"`
	RunID         string            `json:"runId,omitempty"`
	Kind          string            `json:"kind,omitempty"`
	Provider      string            `json:"provider,omitempty"`
	Text          string            `json:"text"`
	Aborted       bool              `json:"aborted,omitempty"`
	ToolCall      *ToolCallRecord   `json:"toolCall,omitempty"`
	Usage         *UsageRecord      `json:"usage,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	At            time.Time         `json:"at"`
}

type ToolCallRecord struct {
	Name   string         `json:"name"`
	Input  map[string]any `json:"input,omitempty"`
	Output any            `json:"output,omitempty"`
	Error  string         `json:"error,omitempty"`
	Status string         `json:"status,omitempty"`
}

type UsageRecord struct {
	InputTokens  int `json:"inputTokens,omitempty"`
	OutputTokens int `json:"outputTokens,omitempty"`
	TotalTokens  int `json:"totalTokens,omitempty"`
}

type AppendOptions struct {
	EventType string
	RunID     string
	Kind      string
	Provider  string
	Aborted   bool
	ToolCall  *ToolCallRecord
	Usage     *UsageRecord
	Metadata  map[string]string
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
	return m.AppendWithOptions(entry, role, text, AppendOptions{})
}

func (m *Manager) AppendWithOptions(entry Entry, role, text string, opts AppendOptions) error {
	if strings.TrimSpace(text) == "" && opts.ToolCall == nil && opts.Usage == nil && len(opts.Metadata) == 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	recordType := "message"
	eventType := strings.TrimSpace(opts.EventType)
	if eventType == "" {
		eventType = "message"
	}
	if eventType != "message" {
		recordType = "event"
	}
	record := MessageRecord{
		SchemaVersion: TranscriptSchemaV2,
		Type:          recordType,
		EventType:     eventType,
		Role:          role,
		SessionID:     entry.SessionID,
		SessionKey:    entry.SessionKey,
		AgentID:       entry.AgentID,
		RunID:         strings.TrimSpace(opts.RunID),
		Kind:          strings.TrimSpace(opts.Kind),
		Provider:      strings.TrimSpace(opts.Provider),
		Text:          text,
		Aborted:       opts.Aborted,
		ToolCall:      opts.ToolCall,
		Usage:         opts.Usage,
		Metadata:      opts.Metadata,
		At:            time.Now().UTC(),
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
		normalizeMessageRecord(&rec, entry)
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

func normalizeMessageRecord(rec *MessageRecord, entry Entry) {
	if rec.SessionID == "" {
		rec.SessionID = entry.SessionID
	}
	if rec.SessionKey == "" {
		rec.SessionKey = entry.SessionKey
	}
	if rec.AgentID == "" {
		rec.AgentID = entry.AgentID
	}
	if rec.Type == "" {
		rec.Type = "message"
	}
	if rec.EventType == "" {
		if rec.Type == "event" {
			rec.EventType = "event"
		} else {
			rec.EventType = "message"
		}
	}
	if rec.SchemaVersion == 0 {
		if rec.RunID != "" || rec.Kind != "" || rec.Provider != "" || rec.ToolCall != nil || rec.Usage != nil || len(rec.Metadata) > 0 || rec.EventType != "message" {
			rec.SchemaVersion = TranscriptSchemaV2
		} else {
			rec.SchemaVersion = 1
		}
	}
}
