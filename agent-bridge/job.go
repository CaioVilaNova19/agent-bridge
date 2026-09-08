package main

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

type JobStatus string

const (
	JobPending JobStatus = "pending"
	JobRunning JobStatus = "running"
	JobDone    JobStatus = "done"
	JobError   JobStatus = "error"
)

// Job is the unit of work behind the async API: POST /v1/chat creates one
// and returns immediately; GET /v1/jobs/{id} polls it until it's done.
// This avoids relying on a single long-lived HTTP connection surviving a
// codex/claude turn that can take minutes.
type Job struct {
	ID              string    `json:"job_id"`
	SessionID       string    `json:"session_id,omitempty"`
	Status          JobStatus `json:"status"`
	Reply           string    `json:"reply,omitempty"`
	EngineSessionID string    `json:"engine_session_id,omitempty"`
	Error           string    `json:"error,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type JobStore struct {
	mu   sync.Mutex
	jobs map[string]*Job
	ttl  time.Duration
}

func NewJobStore(ttl time.Duration) *JobStore {
	s := &JobStore{jobs: make(map[string]*Job), ttl: ttl}
	go s.gcLoop()
	return s
}

func newJobID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "job_" + hex.EncodeToString(buf), nil
}

func (s *JobStore) create(sessionID string) (*Job, error) {
	id, err := newJobID()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	job := &Job{ID: id, SessionID: sessionID, Status: JobPending, CreatedAt: now, UpdatedAt: now}
	s.mu.Lock()
	s.jobs[id] = job
	s.mu.Unlock()
	return job, nil
}

// get returns a copy so callers never race with update() while marshalling.
func (s *JobStore) get(id string) (*Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return nil, false
	}
	cp := *job
	return &cp, true
}

func (s *JobStore) update(id string, mutate func(*Job)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	job, ok := s.jobs[id]
	if !ok {
		return
	}
	mutate(job)
	job.UpdatedAt = time.Now()
}

func (s *JobStore) gcLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-s.ttl)
		s.mu.Lock()
		for id, job := range s.jobs {
			finished := job.Status == JobDone || job.Status == JobError
			if finished && job.UpdatedAt.Before(cutoff) {
				delete(s.jobs, id)
			}
		}
		s.mu.Unlock()
	}
}
