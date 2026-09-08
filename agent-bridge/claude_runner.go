package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// claudeResultEnvelope mirrors the single JSON object printed by
// `claude --print --output-format json` once the turn finishes.
type claudeResultEnvelope struct {
	Type      string `json:"type"`
	Subtype   string `json:"subtype"`
	Result    string `json:"result"`
	SessionID string `json:"session_id"`
	IsError   bool   `json:"is_error"`
}

// ClaudeRunner shells out to the local `claude` CLI (authenticated via the
// user's Claude Pro/Max plan login, no API key involved).
type ClaudeRunner struct {
	Bin            string
	Workdir        string
	PermissionMode string
	AllowedTools   []string
}

func (r *ClaudeRunner) Name() string { return "claude" }

func (r *ClaudeRunner) baseFlags() []string {
	args := []string{"--print", "--output-format", "json", "--permission-mode", r.PermissionMode}
	if len(r.AllowedTools) > 0 {
		args = append(args, "--allowedTools")
		args = append(args, r.AllowedTools...)
	}
	return args
}

func (r *ClaudeRunner) RunNew(ctx context.Context, prompt string) (*RunResult, error) {
	args := r.baseFlags()
	args = append(args, prompt)
	return r.run(ctx, args)
}

func (r *ClaudeRunner) RunResume(ctx context.Context, sessionID, prompt string) (*RunResult, error) {
	args := []string{"--resume", sessionID}
	args = append(args, r.baseFlags()...)
	args = append(args, prompt)
	return r.run(ctx, args)
}

func (r *ClaudeRunner) run(ctx context.Context, args []string) (*RunResult, error) {
	cmd := exec.CommandContext(ctx, r.Bin, args...)
	if r.Workdir != "" {
		cmd.Dir = r.Workdir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, fmt.Errorf("claude exec failed: %s", msg)
	}

	var env claudeResultEnvelope
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &env); err != nil {
		return nil, fmt.Errorf("parsing claude json output: %w (raw: %s)", err, strings.TrimSpace(stdout.String()))
	}

	if env.IsError || (env.Subtype != "" && env.Subtype != "success") {
		msg := env.Result
		if msg == "" {
			msg = fmt.Sprintf("claude reported subtype=%q", env.Subtype)
		}
		return nil, fmt.Errorf("claude turn failed: %s", msg)
	}
	if env.Result == "" {
		return nil, fmt.Errorf("claude produced no reply (stderr: %s)", strings.TrimSpace(stderr.String()))
	}

	return &RunResult{EngineSessionID: env.SessionID, Reply: env.Result}, nil
}
