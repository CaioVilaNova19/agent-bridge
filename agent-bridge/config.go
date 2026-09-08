package main

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type CodexSettings struct {
	Bin            string `json:"bin"`
	Sandbox        string `json:"sandbox"`
	ApprovalPolicy string `json:"approval_policy"`
}

type ClaudeSettings struct {
	Bin            string   `json:"bin"`
	PermissionMode string   `json:"permission_mode"`
	AllowedTools   []string `json:"allowed_tools,omitempty"`
}

// Config is persisted as JSON in ~/.agent-bridge/config.json and can be
// overridden per-field by AGENTBRIDGE_* environment variables at startup.
type Config struct {
	Engine            string         `json:"engine"`
	ListenAddr        string         `json:"listen_addr"`
	AuthToken         string         `json:"auth_token,omitempty"`
	Workdir           string         `json:"workdir"`
	Codex             CodexSettings  `json:"codex"`
	Claude            ClaudeSettings `json:"claude"`
	TimeoutSeconds    int            `json:"timeout_seconds"`
	SessionTTLMinutes int            `json:"session_ttl_minutes"`

	RequestTimeout time.Duration `json:"-"`
	SessionTTL     time.Duration `json:"-"`
}

func defaultConfig() *Config {
	return &Config{
		ListenAddr: "127.0.0.1:8080",
		Workdir:    ".",
		Codex: CodexSettings{
			Bin:            "codex",
			Sandbox:        "read-only",
			ApprovalPolicy: "never",
		},
		Claude: ClaudeSettings{
			Bin:            "claude",
			PermissionMode: "plan",
		},
		TimeoutSeconds:    300,
		SessionTTLMinutes: 60,
	}
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home dir: %w", err)
	}
	return filepath.Join(home, ".agent-bridge", "config.json"), nil
}

// loadConfigFile returns an error satisfying os.IsNotExist when there is
// no saved config yet — callers use that to decide whether to run the wizard.
func loadConfigFile() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := defaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	return cfg, nil
}

func saveConfigFile(cfg *Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("creating config dir: %w", err)
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("encoding config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}
	return nil
}

// applyEnvOverrides lets AGENTBRIDGE_* env vars override the persisted
// config without editing the file — handy for containers/systemd.
func applyEnvOverrides(cfg *Config) {
	if v, ok := os.LookupEnv("AGENTBRIDGE_ENGINE"); ok && v != "" {
		cfg.Engine = v
	}
	if v, ok := os.LookupEnv("AGENTBRIDGE_LISTEN_ADDR"); ok && v != "" {
		cfg.ListenAddr = v
	}
	if v, ok := os.LookupEnv("AGENTBRIDGE_AUTH_TOKEN"); ok {
		cfg.AuthToken = v
	}
	if v, ok := os.LookupEnv("AGENTBRIDGE_WORKDIR"); ok && v != "" {
		cfg.Workdir = v
	}
	if v, ok := os.LookupEnv("AGENTBRIDGE_CODEX_BIN"); ok && v != "" {
		cfg.Codex.Bin = v
	}
	if v, ok := os.LookupEnv("AGENTBRIDGE_CODEX_SANDBOX"); ok && v != "" {
		cfg.Codex.Sandbox = v
	}
	if v, ok := os.LookupEnv("AGENTBRIDGE_CODEX_APPROVAL_POLICY"); ok && v != "" {
		cfg.Codex.ApprovalPolicy = v
	}
	if v, ok := os.LookupEnv("AGENTBRIDGE_CLAUDE_BIN"); ok && v != "" {
		cfg.Claude.Bin = v
	}
	if v, ok := os.LookupEnv("AGENTBRIDGE_CLAUDE_PERMISSION_MODE"); ok && v != "" {
		cfg.Claude.PermissionMode = v
	}
	if v, ok := os.LookupEnv("AGENTBRIDGE_CLAUDE_ALLOWED_TOOLS"); ok && v != "" {
		var tools []string
		for _, t := range strings.Split(v, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tools = append(tools, t)
			}
		}
		cfg.Claude.AllowedTools = tools
	}
	if v, ok := os.LookupEnv("AGENTBRIDGE_TIMEOUT_SECONDS"); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.TimeoutSeconds = n
		}
	}
	if v, ok := os.LookupEnv("AGENTBRIDGE_SESSION_TTL_MINUTES"); ok && v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.SessionTTLMinutes = n
		}
	}
}

func (cfg *Config) finalize() error {
	if cfg.Engine != "codex" && cfg.Engine != "claude" {
		return fmt.Errorf("engine precisa ser \"codex\" ou \"claude\" (valor atual: %q) — rode `agent-bridge init`", cfg.Engine)
	}
	switch cfg.Codex.Sandbox {
	case "read-only", "workspace-write", "danger-full-access":
	default:
		return fmt.Errorf("codex.sandbox inválido: %q", cfg.Codex.Sandbox)
	}
	switch cfg.Claude.PermissionMode {
	case "plan", "acceptEdits", "bypassPermissions":
	default:
		return fmt.Errorf("claude.permission_mode inválido: %q", cfg.Claude.PermissionMode)
	}
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 300
	}
	if cfg.SessionTTLMinutes <= 0 {
		cfg.SessionTTLMinutes = 60
	}
	cfg.RequestTimeout = time.Duration(cfg.TimeoutSeconds) * time.Second
	cfg.SessionTTL = time.Duration(cfg.SessionTTLMinutes) * time.Minute

	if !isLoopback(cfg.ListenAddr) && strings.TrimSpace(cfg.AuthToken) == "" {
		return fmt.Errorf("AGENTBRIDGE_AUTH_TOKEN (ou um token salvo) é obrigatório para bind não-loopback (%s)", cfg.ListenAddr)
	}
	return nil
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
