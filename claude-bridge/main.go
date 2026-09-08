package main

import (
	"log"
	"net/http"
)

func main() {
	cfg, err := loadConfig()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	runner := &ClaudeRunner{
		Bin:            cfg.ClaudeBin,
		Workdir:        cfg.Workdir,
		PermissionMode: cfg.PermissionMode,
		AllowedTools:   cfg.AllowedTools,
	}
	sessions := NewSessionStore(cfg.SessionTTL)
	srv := NewServer(cfg, runner, sessions)

	if cfg.AuthToken == "" {
		log.Printf("warning: CLAUDEBRIDGE_AUTH_TOKEN is not set; this is only safe while bound to loopback (%s)", cfg.ListenAddr)
	}
	log.Printf("claude-bridge listening on %s (permission-mode=%s, workdir=%s)", cfg.ListenAddr, cfg.PermissionMode, cfg.Workdir)

	if err := http.ListenAndServe(cfg.ListenAddr, srv.Handler()); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
