package sessions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Entry struct {
	SessionKey string    `json:"sessionKey"`
	SessionID  string    `json:"sessionId"`
	AgentID    string    `json:"agentId"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type Store interface {
	Get(key string) (Entry, bool)
	Put(entry Entry) error
	List() []Entry
}

type InMemoryStore struct {
	mu   sync.RWMutex
	data map[string]Entry
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{data: make(map[string]Entry)}
}

func (s *InMemoryStore) Get(key string) (Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.data[key]
	return e, ok
}

func (s *InMemoryStore) Put(entry Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[entry.SessionKey] = entry
	return nil
}

func (s *InMemoryStore) List() []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Entry, 0, len(s.data))
	for _, e := range s.data {
		out = append(out, e)
	}
	return out
}

type FileStore struct {
	mu   sync.RWMutex
	path string
	data map[string]Entry
}

func NewFileStore(path string) (*FileStore, error) {
	fs := &FileStore{
		path: path,
		data: make(map[string]Entry),
	}
	if err := fs.load(); err != nil {
		return nil, err
	}
	return fs, nil
}

func (s *FileStore) load() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	b, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(b) == 0 {
		return nil
	}
	if err := json.Unmarshal(b, &s.data); err != nil {
		return fmt.Errorf("sessions: parse store: %w", err)
	}
	return nil
}

func (s *FileStore) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *FileStore) Get(key string) (Entry, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.data[key]
	return e, ok
}

func (s *FileStore) Put(entry Entry) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[entry.SessionKey] = entry
	return s.saveLocked()
}

func (s *FileStore) List() []Entry {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Entry, 0, len(s.data))
	for _, e := range s.data {
		out = append(out, e)
	}
	return out
}
