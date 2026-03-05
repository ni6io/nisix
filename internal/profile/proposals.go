package profile

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type proposalStore struct {
	mu        sync.Mutex
	proposals map[string]Proposal
	ttl       time.Duration
}

func newProposalStore(ttl time.Duration) *proposalStore {
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	return &proposalStore{
		proposals: make(map[string]Proposal),
		ttl:       ttl,
	}
}

func (s *proposalStore) create(sessionKey string, req UpdateRequest, summary string) Proposal {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	id := fmt.Sprintf("prop-%d", now.UnixNano())
	p := Proposal{
		ID:         id,
		SessionKey: sessionKey,
		Request:    req,
		Summary:    summary,
		CreatedAt:  now,
		ExpiresAt:  now.Add(s.ttl),
	}
	s.proposals[id] = p
	return p
}

func (s *proposalStore) get(id string) (Proposal, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.proposals[id]
	if !ok {
		return Proposal{}, false
	}
	if time.Now().UTC().After(p.ExpiresAt) {
		delete(s.proposals, id)
		return Proposal{}, false
	}
	return p, true
}

func (s *proposalStore) delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.proposals, id)
}

func (s *proposalStore) latest(sessionKey string, file string) (Proposal, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	targetSession := strings.TrimSpace(sessionKey)
	targetFile := strings.ToUpper(strings.TrimSpace(file))
	var out Proposal
	found := false
	for id, p := range s.proposals {
		if now.After(p.ExpiresAt) {
			delete(s.proposals, id)
			continue
		}
		if strings.TrimSpace(p.SessionKey) != targetSession {
			continue
		}
		if strings.ToUpper(strings.TrimSpace(p.Request.File)) != targetFile {
			continue
		}
		if !found || p.CreatedAt.After(out.CreatedAt) {
			out = p
			found = true
		}
	}
	return out, found
}
