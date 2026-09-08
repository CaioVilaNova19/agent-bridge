package main

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime settings, sourced from environment variables so
// the same binary works unchanged in WSL (loopback only) or on a VPS
// (public bind + mandatory auth token).
type Config struct {
	ListenAddr     string
	AuthToken      string
	ClaudeBin      string
	Workdir        string
	PermissionMode string
	AllowedTools   []string
	RequestTimeout time.Duration
	SessionTTL     time.Duration
}

func loadConfig() (Config, error) {
	cfg := Config{
		ListenAddr:     getEnv("CLAUDEBRIDGE_LISTEN_ADDR", "127.0.0.1:8081"),
		AuthToken:      os.Getenv("CLAUDEBRIDGE_AUTH_TOKEN"),
		ClaudeBin:      getEnv("CLAUDEBRIDGE_CLAUDE_BIN", "claude"),
		Workdir:        getEnv("CLAUDEBRIDGE_WORKDIR", "."),
		PermissionMode: getEnv("CLAUDEBRIDGE_PERMISSION_MODE", "plan"),
	}

	if raw := strings.TrimSpace(os.Getenv("CLAUDEBRIDGE_ALLOWED_TOOLS")); raw != "" {
		for _, tool := range strings.Split(raw, ",") {
			tool = strings.TrimSpace(tool)
			if tool != "" {
				cfg.AllowedTools = append(cfg.AllowedTools, tool)
			}
		}
	}

	timeoutSec, err := strconv.Atoi(getEnv("CLAUDEBRIDGE_TIMEOUT_SECONDS", "300"))
	if err != nil {
		return cfg, fmt.Errorf("invalid CLAUDEBRIDGE_TIMEOUT_SECONDS: %w", err)
	}
	cfg.RequestTimeout = time.Duration(timeoutSec) * time.Second

	ttlMin, err := strconv.Atoi(getEnv("CLAUDEBRIDGE_SESSION_TTL_MINUTES", "60"))
	if err != nil {
		return cfg, fmt.Errorf("invalid CLAUDEBRIDGE_SESSION_TTL_MINUTES: %w", err)
	}
	cfg.SessionTTL = time.Duration(ttlMin) * time.Minute

	switch cfg.PermissionMode {
	case "plan", "acceptEdits", "bypassPermissions":
	default:
		return cfg, fmt.Errorf("invalid CLAUDEBRIDGE_PERMISSION_MODE %q (use plan|acceptEdits|bypassPermissions)", cfg.PermissionMode)
	}

	if !isLoopback(cfg.ListenAddr) && strings.TrimSpace(cfg.AuthToken) == "" {
		return cfg, fmt.Errorf("CLAUDEBRIDGE_AUTH_TOKEN is required when binding to a non-loopback address (%s); set it or bind to 127.0.0.1", cfg.ListenAddr)
	}

	return cfg, nil
}

func getEnv(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

// isLoopback reports whether addr only accepts local connections. An empty
// host (":8081") or a wildcard IP binds every interface and is NOT loopback.
func isLoopback(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}
