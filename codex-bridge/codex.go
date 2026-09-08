package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// execItem mirrors the "item" object inside item.completed events emitted
// by `codex exec --json`.
type execItem struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Text string `json:"text"`
}

// execEvent is a single NDJSON line produced by `codex exec --json`.
// Only the fields codex-bridge cares about are decoded; unknown event
// types are ignored.
type execEvent struct {
	Type     string    `json:"type"`
	ThreadID string    `json:"thread_id"`
	Item     *execItem `json:"item"`
	Error    *struct {
		Message string `json:"message"`
	} `json:"error"`
}

// CodexRunner shells out to the local `codex` CLI (authenticated via the
// user's ChatGPT plan login, no API key involved).
type CodexRunner struct {
	Bin            string
	Workdir        string
	Sandbox        string
	ApprovalPolicy string
}

type CodexResult struct {
	ThreadID string
	Reply    string
}

func (r *CodexRunner) baseFlags() []string {
	return []string{
		"--sandbox", r.Sandbox,
		"--ask-for-approval", r.ApprovalPolicy,
		"--json",
	}
}

// RunNew starts a brand new codex thread for prompt.
func (r *CodexRunner) RunNew(ctx context.Context, prompt string) (*CodexResult, error) {
	args := []string{"exec"}
	args = append(args, r.baseFlags()...)
	args = append(args, "-C", r.Workdir, prompt)
	return r.run(ctx, args)
}

// RunResume continues an existing thread so codex keeps prior context.
func (r *CodexRunner) RunResume(ctx context.Context, threadID, prompt string) (*CodexResult, error) {
	args := []string{"exec", "resume", threadID}
	args = append(args, r.baseFlags()...)
	args = append(args, prompt)
	return r.run(ctx, args)
}

func (r *CodexRunner) run(ctx context.Context, args []string) (*CodexResult, error) {
	cmd := exec.CommandContext(ctx, r.Bin, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("stdout pipe: %w", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start codex: %w", err)
	}

	result := &CodexResult{}
	var turnErr error

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var ev execEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			// codex may print non-JSON diagnostics even with --json; skip them.
			continue
		}
		switch {
		case ev.Type == "thread.started" && ev.ThreadID != "":
			result.ThreadID = ev.ThreadID
		case ev.Type == "item.completed" && ev.Item != nil && ev.Item.Type == "agent_message":
			result.Reply = ev.Item.Text
		case ev.Type == "turn.failed":
			if ev.Error != nil && ev.Error.Message != "" {
				turnErr = fmt.Errorf("codex turn failed: %s", ev.Error.Message)
			} else {
				turnErr = fmt.Errorf("codex turn failed")
			}
		}
	}
	scanErr := scanner.Err()
	waitErr := cmd.Wait()

	if waitErr != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = waitErr.Error()
		}
		return nil, fmt.Errorf("codex exec failed: %s", msg)
	}
	if scanErr != nil {
		return nil, fmt.Errorf("reading codex output: %w", scanErr)
	}
	if turnErr != nil {
		return nil, turnErr
	}
	if result.Reply == "" {
		return nil, fmt.Errorf("codex produced no reply (stderr: %s)", strings.TrimSpace(stderr.String()))
	}
	return result, nil
}
