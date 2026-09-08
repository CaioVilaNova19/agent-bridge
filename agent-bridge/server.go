package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"
)

type ChatRequest struct {
	SessionID string `json:"session_id"`
	Prompt    string `json:"prompt"`
}

type apiError struct {
	Error string `json:"error"`
}

type Server struct {
	cfg      *Config
	runner   Runner
	sessions *SessionStore
	jobs     *JobStore
}

func NewServer(cfg *Config, runner Runner, sessions *SessionStore, jobs *JobStore) *Server {
	return &Server{cfg: cfg, runner: runner, sessions: sessions, jobs: jobs}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/v1/info", s.handleInfo)
	mux.HandleFunc("POST /v1/chat", s.authMiddleware(s.handleChat))
	mux.HandleFunc("GET /v1/jobs/{id}", s.authMiddleware(s.handleJobStatus))
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) handleInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"engine":   s.runner.Name(),
		"api_docs": "POST /v1/chat -> {job_id}; GET /v1/jobs/{job_id} -> status/reply",
	})
}

func (s *Server) authMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.AuthToken != "" {
			header := r.Header.Get("Authorization")
			token := strings.TrimPrefix(header, "Bearer ")
			if token == header || subtle.ConstantTimeCompare([]byte(token), []byte(s.cfg.AuthToken)) != 1 {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next(w, r)
	}
}

// handleChat never waits for codex/claude: it creates a job and returns
// immediately, because a real turn can take minutes and most HTTP clients
// (and reverse proxies) give up long before that.
func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "invalid json body: " + err.Error()})
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		writeJSON(w, http.StatusBadRequest, apiError{Error: "prompt is required"})
		return
	}

	job, err := s.jobs.create(req.SessionID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, apiError{Error: "could not create job: " + err.Error()})
		return
	}

	go s.processJob(job.ID, req.SessionID, req.Prompt)

	writeJSON(w, http.StatusAccepted, job)
}

func (s *Server) processJob(jobID, sessionID, prompt string) {
	s.jobs.update(jobID, func(j *Job) { j.Status = JobRunning })

	// Deliberately not r.Context(): the HTTP request that created this job
	// already got its response and may be long gone by the time this runs.
	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.RequestTimeout)
	defer cancel()

	var result *RunResult
	var err error

	if sessionID == "" {
		result, err = s.runner.RunNew(ctx, prompt)
	} else {
		sess := s.sessions.get(sessionID)
		sess.mu.Lock()
		defer sess.mu.Unlock()

		if sess.EngineSessionID == "" {
			result, err = s.runner.RunNew(ctx, prompt)
		} else {
			result, err = s.runner.RunResume(ctx, sess.EngineSessionID, prompt)
		}
		if err == nil && result.EngineSessionID != "" {
			sess.EngineSessionID = result.EngineSessionID
		}
		sess.LastUsedAt = time.Now()
	}

	if err != nil {
		log.Printf("%s error (session=%q, job=%s): %v", s.runner.Name(), sessionID, jobID, err)
		s.jobs.update(jobID, func(j *Job) {
			j.Status = JobError
			j.Error = err.Error()
		})
		return
	}

	s.jobs.update(jobID, func(j *Job) {
		j.Status = JobDone
		j.Reply = result.Reply
		j.EngineSessionID = result.EngineSessionID
	})
}

func (s *Server) handleJobStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	job, ok := s.jobs.get(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, apiError{Error: "job not found (id errado ou expirou)"})
		return
	}
	writeJSON(w, http.StatusOK, job)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
