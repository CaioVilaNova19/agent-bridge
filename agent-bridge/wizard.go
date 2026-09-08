package main

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type wizard struct {
	in  *bufio.Reader
	out *os.File
}

func newWizard() *wizard {
	return &wizard{in: bufio.NewReader(os.Stdin), out: os.Stdout}
}

func (w *wizard) printf(format string, a ...interface{}) {
	fmt.Fprintf(w.out, format, a...)
}

func (w *wizard) ask(prompt, def string) string {
	if def != "" {
		w.printf("%s [%s]: ", prompt, def)
	} else {
		w.printf("%s: ", prompt)
	}
	line, _ := w.in.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

func (w *wizard) askYesNo(prompt string, def bool) bool {
	suffix := "s/N"
	if def {
		suffix = "S/n"
	}
	line := strings.ToLower(w.ask(fmt.Sprintf("%s (%s)", prompt, suffix), ""))
	if line == "" {
		return def
	}
	return line == "s" || line == "sim" || line == "y" || line == "yes"
}

func runInteractive(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func randomToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// runWizard is the first-run onboarding: detect what's installed, help
// with login, ask where this will run, and persist the result so future
// runs skip straight to serving.
func runWizard() (*Config, error) {
	w := newWizard()
	w.printf("\n=== agent-bridge - configuracao inicial ===\n\n")
	w.printf("Isso sobe um servidor local que expoe o Codex CLI ou o Claude Code\n")
	w.printf("(autenticados com sua assinatura, sem API key) numa porta HTTP.\n\n")

	cfg := defaultConfig()

	var det detection
	for {
		det = detectEngines()
		if det.CodexFound || det.ClaudeFound {
			break
		}
		w.printf("Nao encontrei `codex` nem `claude` no PATH.\n\n")
		w.printf("Instale um deles e volte aqui (nao vou instalar nada sozinho):\n\n")
		w.printf("  Codex CLI:    npm install -g @openai/codex\n")
		w.printf("                (ou: curl -fsSL https://chatgpt.com/codex/install.sh | sh)\n")
		w.printf("  Claude Code:  curl -fsSL https://claude.ai/install.sh | bash\n")
		w.printf("                (ou: npm install -g @anthropic-ai/claude-code)\n\n")
		if !w.askYesNo("Ja instalou e quer que eu verifique de novo?", true) {
			return nil, fmt.Errorf("nenhuma CLI de agente encontrada no PATH")
		}
	}

	w.printf("Encontrado:\n")
	if det.CodexFound {
		w.printf("  [OK] codex   -> %s\n", det.CodexBin)
	} else {
		w.printf("  [--] codex   (nao encontrado)\n")
	}
	if det.ClaudeFound {
		w.printf("  [OK] claude  -> %s\n", det.ClaudeBin)
	} else {
		w.printf("  [--] claude  (nao encontrado)\n")
	}
	w.printf("\n")

	switch {
	case det.CodexFound && !det.ClaudeFound:
		cfg.Engine = "codex"
		w.printf("Usando codex (foi o unico encontrado).\n\n")
	case det.ClaudeFound && !det.CodexFound:
		cfg.Engine = "claude"
		w.printf("Usando claude (foi o unico encontrado).\n\n")
	default:
		for {
			choice := w.ask("Qual usar? [1] codex  [2] claude", "1")
			if choice == "1" || strings.EqualFold(choice, "codex") {
				cfg.Engine = "codex"
				break
			}
			if choice == "2" || strings.EqualFold(choice, "claude") {
				cfg.Engine = "claude"
				break
			}
			w.printf("Nao entendi, digite 1 ou 2.\n")
		}
		w.printf("\n")
	}

	if cfg.Engine == "codex" {
		cfg.Codex.Bin = det.CodexBin
		for !codexLoggedIn() {
			w.printf("Nao encontrei login salvo do Codex (~/.codex/auth.json).\n")
			if w.askYesNo("Rodar `codex login` agora?", true) {
				if err := runInteractive(det.CodexBin, "login"); err != nil {
					w.printf("`codex login` retornou erro: %v\n", err)
				}
				continue
			}
			w.printf("Ok — rode `codex login` manualmente antes de usar o servidor.\n")
			break
		}
	} else {
		cfg.Claude.Bin = det.ClaudeBin
		if !w.askYesNo("Voce ja fez login no Claude Code (rodou `claude` e usou /login)?", true) {
			if w.askYesNo("Abrir o `claude` agora para voce fazer login?", true) {
				if err := runInteractive(det.ClaudeBin); err != nil {
					w.printf("`claude` retornou erro: %v\n", err)
				}
			} else {
				w.printf("Ok — faca login manualmente antes de usar o servidor.\n")
			}
		}
	}
	w.printf("\n")

	vps := w.ask("Onde isso vai rodar? [1] so local (WSL/localhost)  [2] VPS exposta na rede", "1") == "2"
	port := w.ask("Porta", "8080")
	if _, err := strconv.Atoi(port); err != nil {
		port = "8080"
	}
	if vps {
		cfg.ListenAddr = "0.0.0.0:" + port
		token, err := randomToken()
		if err != nil {
			return nil, fmt.Errorf("gerando token: %w", err)
		}
		cfg.AuthToken = token
		w.printf("\nToken de autenticacao gerado (fica salvo no config, nao vai\n")
		w.printf("aparecer de novo aqui — guarde agora):\n\n  %s\n\n", token)
		w.printf("Coloque isso atras de um reverse proxy com TLS e restrinja o\n")
		w.printf("firewall por IP — o token sozinho nao basta na internet aberta.\n\n")
	} else {
		cfg.ListenAddr = "127.0.0.1:" + port
	}

	if cfg.Engine == "codex" {
		if w.askYesNo("Permitir que o agente escreva arquivos no diretorio de trabalho?", false) {
			cfg.Codex.Sandbox = "workspace-write"
		}
	} else {
		if w.askYesNo("Permitir que o agente edite arquivos no diretorio de trabalho?", false) {
			cfg.Claude.PermissionMode = "acceptEdits"
		}
	}

	cfg.Workdir = w.ask("Diretorio de trabalho do agente", ".")

	if err := saveConfigFile(cfg); err != nil {
		return nil, err
	}
	path, _ := configPath()
	w.printf("\nConfiguracao salva em %s\n", path)

	return cfg, nil
}

func askStartNow() bool {
	return newWizard().askYesNo("Iniciar o servidor agora?", true)
}
