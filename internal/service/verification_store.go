package service

import (
	"sync"
	"time"
)

type verificationEntry struct {
	code      string
	expiresAt time.Time
}

type VerificationStore struct {
	mu    sync.Mutex
	codes map[string]verificationEntry
	ttl   time.Duration
}

func NewVerificationStore(ttl time.Duration) *VerificationStore {
	return &VerificationStore{
		codes: make(map[string]verificationEntry),
		ttl:   ttl,
	}
}

func (s *VerificationStore) Set(email, code string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.codes[email] = verificationEntry{
		code:      code,
		expiresAt: time.Now().Add(s.ttl),
	}
}

func (s *VerificationStore) Verify(email, code string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry, ok := s.codes[email]
	if !ok {
		return false
	}
	if time.Now().After(entry.expiresAt) {
		delete(s.codes, email)
		return false
	}
	if entry.code != code {
		return false
	}
	delete(s.codes, email)
	return true
}
