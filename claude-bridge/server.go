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

type ChatResponse struct {
	SessionID       string `json:"session_id,omitempty"`
	ClaudeSessionID string `json:"claude_session_id,omitempty"`
	Reply           string `json:"reply,omitempty"`
	Error           string `json:"error,omitempty"`
}

type Server struct {
	cfg      Config
	runner   *ClaudeRunner
	sessions *SessionStore
}

func NewServer(cfg Config, runner *ClaudeRunner, sessions *SessionStore) *Server {
	return &Server{cfg: cfg, runner: runner, sessions: sessions}
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealth)
	mux.HandleFunc("/v1/chat", s.authMiddleware(s.handleChat))
	return mux
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
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

func (s *Server) handleChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, ChatResponse{Error: "invalid json body: " + err.Error()})
		return
	}
	if strings.TrimSpace(req.Prompt) == "" {
		writeJSON(w, http.StatusBadRequest, ChatResponse{Error: "prompt is required"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), s.cfg.RequestTimeout)
	defer cancel()

	var result *ClaudeResult
	var err error

	if req.SessionID == "" {
		result, err = s.runner.RunNew(ctx, req.Prompt)
	} else {
		sess := s.sessions.get(req.SessionID)
		sess.mu.Lock()
		defer sess.mu.Unlock()

		if sess.ClaudeSessID == "" {
			result, err = s.runner.RunNew(ctx, req.Prompt)
		} else {
			result, err = s.runner.RunResume(ctx, sess.ClaudeSessID, req.Prompt)
		}
		if err == nil && result.SessionID != "" {
			sess.ClaudeSessID = result.SessionID
		}
		sess.LastUsedAt = time.Now()
	}

	if err != nil {
		log.Printf("claude error (session=%q): %v", req.SessionID, err)
		writeJSON(w, http.StatusBadGateway, ChatResponse{SessionID: req.SessionID, Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, ChatResponse{
		SessionID:       req.SessionID,
		ClaudeSessionID: result.SessionID,
		Reply:           result.Reply,
	})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
