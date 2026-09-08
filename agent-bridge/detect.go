package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

type detection struct {
	CodexFound  bool
	ClaudeFound bool
	CodexBin    string
	ClaudeBin   string
}

func detectEngines() detection {
	var d detection
	if p, err := exec.LookPath("codex"); err == nil {
		d.CodexFound = true
		d.CodexBin = p
	}
	if p, err := exec.LookPath("claude"); err == nil {
		d.ClaudeFound = true
		d.ClaudeBin = p
	}
	return d
}

// codexLoggedIn checks for the auth cache codex writes after `codex login`.
// There is no equivalent public, stable path documented for Claude Code,
// so that engine is confirmed by asking the user instead (see wizard.go).
func codexLoggedIn() bool {
	home, err := os.UserHomeDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(home, ".codex", "auth.json"))
	return err == nil
}
