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

	runner := &CodexRunner{
		Bin:            cfg.CodexBin,
		Workdir:        cfg.Workdir,
		Sandbox:        cfg.Sandbox,
		ApprovalPolicy: cfg.ApprovalPolicy,
	}
	sessions := NewSessionStore(cfg.SessionTTL)
	srv := NewServer(cfg, runner, sessions)

	if cfg.AuthToken == "" {
		log.Printf("warning: CODEXBRIDGE_AUTH_TOKEN is not set; this is only safe while bound to loopback (%s)", cfg.ListenAddr)
	}
	log.Printf("codex-bridge listening on %s (sandbox=%s, approval=%s, workdir=%s)", cfg.ListenAddr, cfg.Sandbox, cfg.ApprovalPolicy, cfg.Workdir)

	if err := http.ListenAndServe(cfg.ListenAddr, srv.Handler()); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
