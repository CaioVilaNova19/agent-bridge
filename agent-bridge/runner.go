package main

import "context"

// RunResult is what any engine (codex, claude, ...) returns for one turn.
type RunResult struct {
	EngineSessionID string
	Reply           string
}

// Runner abstracts a single agent CLI so the HTTP server doesn't care
// whether it's talking to codex or claude underneath.
type Runner interface {
	Name() string
	RunNew(ctx context.Context, prompt string) (*RunResult, error)
	RunResume(ctx context.Context, sessionID, prompt string) (*RunResult, error)
}
