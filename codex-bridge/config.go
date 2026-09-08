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
	CodexBin       string
	Workdir        string
	Sandbox        string
	ApprovalPolicy string
	RequestTimeout time.Duration
	SessionTTL     time.Duration
}

func loadConfig() (Config, error) {
	cfg := Config{
		ListenAddr:     getEnv("CODEXBRIDGE_LISTEN_ADDR", "127.0.0.1:8080"),
		AuthToken:      os.Getenv("CODEXBRIDGE_AUTH_TOKEN"),
		CodexBin:       getEnv("CODEXBRIDGE_CODEX_BIN", "codex"),
		Workdir:        getEnv("CODEXBRIDGE_WORKDIR", "."),
		Sandbox:        getEnv("CODEXBRIDGE_SANDBOX", "read-only"),
		ApprovalPolicy: getEnv("CODEXBRIDGE_APPROVAL_POLICY", "never"),
	}

	timeoutSec, err := strconv.Atoi(getEnv("CODEXBRIDGE_TIMEOUT_SECONDS", "300"))
	if err != nil {
		return cfg, fmt.Errorf("invalid CODEXBRIDGE_TIMEOUT_SECONDS: %w", err)
	}
	cfg.RequestTimeout = time.Duration(timeoutSec) * time.Second

	ttlMin, err := strconv.Atoi(getEnv("CODEXBRIDGE_SESSION_TTL_MINUTES", "60"))
	if err != nil {
		return cfg, fmt.Errorf("invalid CODEXBRIDGE_SESSION_TTL_MINUTES: %w", err)
	}
	cfg.SessionTTL = time.Duration(ttlMin) * time.Minute

	switch cfg.Sandbox {
	case "read-only", "workspace-write", "danger-full-access":
	default:
		return cfg, fmt.Errorf("invalid CODEXBRIDGE_SANDBOX %q (use read-only|workspace-write|danger-full-access)", cfg.Sandbox)
	}

	if !isLoopback(cfg.ListenAddr) && strings.TrimSpace(cfg.AuthToken) == "" {
		return cfg, fmt.Errorf("CODEXBRIDGE_AUTH_TOKEN is required when binding to a non-loopback address (%s); set it or bind to 127.0.0.1", cfg.ListenAddr)
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
// host (":8080") or a wildcard IP binds every interface and is NOT loopback.
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
