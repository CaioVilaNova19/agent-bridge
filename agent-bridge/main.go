package main

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
)

func main() {
	cmd := "start"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}

	switch cmd {
	case "doctor":
		runDoctor()
	case "init":
		cfg, err := runWizard()
		if err != nil {
			log.Fatalf("configuracao cancelada: %v", err)
		}
		fmt.Println()
		applyEnvOverrides(cfg)
		if err := cfg.finalize(); err != nil {
			log.Fatalf("%v", err)
		}
		if askStartNow() {
			startServer(cfg)
		}
	case "serve":
		cfg, err := loadOrFailConfig()
		if err != nil {
			log.Fatalf("%v", err)
		}
		startServer(cfg)
	case "start", "":
		cfg, err := loadOrRunWizard()
		if err != nil {
			log.Fatalf("%v", err)
		}
		startServer(cfg)
	case "help", "-h", "--help":
		printHelp()
	default:
		printHelp()
		os.Exit(1)
	}
}

func printHelp() {
	fmt.Println(`agent-bridge - servidor HTTP que expoe Codex CLI ou Claude Code (via login de assinatura) numa porta.

Uso:
  agent-bridge            inicia (roda o assistente de configuracao na primeira vez)
  agent-bridge init       (re)configura interativamente
  agent-bridge serve      inicia sem perguntar nada (usa config salva + variaveis de ambiente)
  agent-bridge doctor     verifica instalacao, login e configuracao
  agent-bridge help       mostra esta ajuda`)
}

func loadOrRunWizard() (*Config, error) {
	cfg, err := loadConfigFile()
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		cfg, err = runWizard()
		if err != nil {
			return nil, err
		}
	}
	applyEnvOverrides(cfg)
	if err := cfg.finalize(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func loadOrFailConfig() (*Config, error) {
	cfg, err := loadConfigFile()
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("nenhuma configuracao encontrada — rode `agent-bridge init` primeiro")
		}
		return nil, err
	}
	applyEnvOverrides(cfg)
	if err := cfg.finalize(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func startServer(cfg *Config) {
	var runner Runner
	switch cfg.Engine {
	case "codex":
		runner = &CodexRunner{
			Bin:            cfg.Codex.Bin,
			Workdir:        cfg.Workdir,
			Sandbox:        cfg.Codex.Sandbox,
			ApprovalPolicy: cfg.Codex.ApprovalPolicy,
		}
	case "claude":
		runner = &ClaudeRunner{
			Bin:            cfg.Claude.Bin,
			Workdir:        cfg.Workdir,
			PermissionMode: cfg.Claude.PermissionMode,
			AllowedTools:   cfg.Claude.AllowedTools,
		}
	default:
		log.Fatalf("engine desconhecido: %q", cfg.Engine)
	}

	sessions := NewSessionStore(cfg.SessionTTL)
	jobs := NewJobStore(cfg.SessionTTL)
	srv := NewServer(cfg, runner, sessions, jobs)

	printStartupBanner(cfg)

	if err := http.ListenAndServe(cfg.ListenAddr, srv.Handler()); err != nil {
		log.Fatalf("erro no servidor: %v", err)
	}
}

func printStartupBanner(cfg *Config) {
	url := "http://" + displayAddr(cfg.ListenAddr)
	line := strings.Repeat("=", 60)

	fmt.Println()
	fmt.Println(line)
	fmt.Printf("  agent-bridge no ar — engine: %s\n", cfg.Engine)
	fmt.Printf("  PORTA / URL DA API: %s\n", url)
	fmt.Println(line)
	if cfg.AuthToken == "" {
		fmt.Println("  aviso: sem token de autenticacao — so e seguro em bind local (loopback).")
	} else {
		fmt.Println("  autenticacao: header 'Authorization: Bearer <token>' obrigatorio")
	}
	fmt.Println()
	fmt.Println("  A API e assincrona: primeiro voce pede, depois voce busca a resposta.")
	fmt.Println()
	fmt.Println("  1) pedir:")
	fmt.Printf("     curl -s %s%s/v1/chat -H 'Content-Type: application/json' -d '{\"prompt\":\"oi\"}'\n",
		authHeaderHint(cfg), url)
	fmt.Println("     -> devolve {\"job_id\":\"job_...\", \"status\":\"pending\", ...}")
	fmt.Println()
	fmt.Println("  2) buscar a resposta (repita ate status virar \"done\"):")
	fmt.Printf("     curl -s %s%s/v1/jobs/job_SEU_ID_AQUI\n", authHeaderHint(cfg), url)
	fmt.Println()
}

func authHeaderHint(cfg *Config) string {
	if cfg.AuthToken == "" {
		return ""
	}
	return fmt.Sprintf("-H 'Authorization: Bearer %s' ", cfg.AuthToken)
}

func displayAddr(addr string) string {
	if strings.HasPrefix(addr, "0.0.0.0:") {
		return "127.0.0.1:" + strings.TrimPrefix(addr, "0.0.0.0:")
	}
	return addr
}
