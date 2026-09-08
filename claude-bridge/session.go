package main

import (
	"sync"
	"time"
)

// Session maps one client-chosen session_id to the underlying Claude Code
// session id, and serializes calls so two requests for the same session
// never race a --resume against each other.
type Session struct {
	mu           sync.Mutex
	ClaudeSessID string
	LastUsedAt   time.Time
}

type SessionStore struct {
	mu       sync.Mutex
	sessions map[string]*Session
	ttl      time.Duration
}

func NewSessionStore(ttl time.Duration) *SessionStore {
	s := &SessionStore{
		sessions: make(map[string]*Session),
		ttl:      ttl,
	}
	go s.gcLoop()
	return s
}

func (s *SessionStore) get(id string) *Session {
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[id]
	if !ok {
		sess = &Session{}
		s.sessions[id] = sess
	}
	return sess
}

func (s *SessionStore) gcLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-s.ttl)
		s.mu.Lock()
		for id, sess := range s.sessions {
			sess.mu.Lock()
			expired := !sess.LastUsedAt.IsZero() && sess.LastUsedAt.Before(cutoff)
			sess.mu.Unlock()
			if expired {
				delete(s.sessions, id)
			}
		}
		s.mu.Unlock()
	}
}
