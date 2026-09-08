package main

import (
	"fmt"
	"os"
	"os/exec"
)

func runDoctor() {
	fmt.Println("=== agent-bridge doctor ===")

	det := detectEngines()
	printCheck("codex no PATH", det.CodexFound, det.CodexBin)
	if det.CodexFound {
		printCheck("codex login (~/.codex/auth.json)", codexLoggedIn(), "")
		printVersion(det.CodexBin, "--version")
	}
	printCheck("claude no PATH", det.ClaudeFound, det.ClaudeBin)
	if det.ClaudeFound {
		printVersion(det.ClaudeBin, "--version")
	}

	path, err := configPath()
	if err != nil {
		return
	}
	cfg, err := loadConfigFile()
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Printf("\nConfig: %s (nao encontrado — rode `agent-bridge init`)\n", path)
		} else {
			fmt.Printf("\nConfig: %s (erro ao ler: %v)\n", path, err)
		}
		return
	}
	applyEnvOverrides(cfg)
	fmt.Printf("\nConfig: %s\n", path)
	fmt.Printf("  engine: %s\n", cfg.Engine)
	fmt.Printf("  URL da API: http://%s\n", cfg.ListenAddr)
	fmt.Printf("  auth token definido: %v\n", cfg.AuthToken != "")
	if err := cfg.finalize(); err != nil {
		fmt.Printf("  [!] configuracao invalida: %v\n", err)
	}
}

func printCheck(label string, ok bool, extra string) {
	mark := "[--]"
	if ok {
		mark = "[OK]"
	}
	if extra != "" {
		fmt.Printf("%s %s (%s)\n", mark, label, extra)
	} else {
		fmt.Printf("%s %s\n", mark, label)
	}
}

func printVersion(bin string, args ...string) {
	out, err := exec.Command(bin, args...).CombinedOutput()
	if err != nil {
		fmt.Printf("      versao: erro (%v)\n", err)
		return
	}
	fmt.Printf("      versao: %s\n", trimOneLine(string(out)))
}

func trimOneLine(s string) string {
	for i, c := range s {
		if c == '\n' || c == '\r' {
			return s[:i]
		}
	}
	return s
}
