# claude-bridge

Igual ao `codex-bridge`, mas usando o Claude Code CLI (login da assinatura
Claude Pro/Max, sem API key) como motor. Mesma ideia: servidor HTTP local
que traduz `POST /v1/chat` em chamadas ao CLI, com sessão persistente.

Não é um proxy passivo: o Claude Code é um agente que pode ler/editar
arquivos e rodar comandos, dependendo do `--permission-mode`. Por isso o
padrão é `plan` (somente leitura/planejamento, não executa nada).

## Pré-requisitos

1. `claude` (Claude Code) instalado e no PATH.
2. Login feito uma vez, interativamente: `claude` e depois `/login` (usa
   sua assinatura Claude Pro/Max; fica em cache localmente, o servidor não
   pede login de novo).
3. Go 1.22+ para compilar.

## Build

```bash
cd claude-bridge
go build -o claude-bridge .
```

## Rodar no WSL (uso local, a partir do Windows)

```bash
./claude-bridge
```

Sobe em `127.0.0.1:8081` por padrão, `--permission-mode plan` (não edita
nada, só lê e responde), sem exigir token — seguro porque só aceita
conexão local.

## Rodar numa VPS (exposto na rede)

```bash
export CLAUDEBRIDGE_LISTEN_ADDR=0.0.0.0:8081
export CLAUDEBRIDGE_AUTH_TOKEN="um-token-longo-e-aleatorio"
export CLAUDEBRIDGE_PERMISSION_MODE=acceptEdits   # só se precisar que ele edite arquivos
./claude-bridge
```

Igual ao codex-bridge: se `CLAUDEBRIDGE_LISTEN_ADDR` não for loopback, o
servidor recusa subir sem `CLAUDEBRIDGE_AUTH_TOKEN`. Numa VPS real, coloque
atrás de reverse proxy com TLS e firewall por IP — o token sozinho não
basta na internet aberta.

## Variáveis de ambiente

| Variável | Padrão | Descrição |
|---|---|---|
| `CLAUDEBRIDGE_LISTEN_ADDR` | `127.0.0.1:8081` | endereço:porta do listener |
| `CLAUDEBRIDGE_AUTH_TOKEN` | (vazio) | token Bearer exigido no header `Authorization`; obrigatório se o bind não for loopback |
| `CLAUDEBRIDGE_CLAUDE_BIN` | `claude` | caminho/nome do binário do CLI |
| `CLAUDEBRIDGE_WORKDIR` | `.` | diretório de trabalho do processo (`cmd.Dir`) |
| `CLAUDEBRIDGE_PERMISSION_MODE` | `plan` | `plan` (somente leitura) \| `acceptEdits` (auto-aceita edição de arquivo) \| `bypassPermissions` (acesso total, sem perguntas) |
| `CLAUDEBRIDGE_ALLOWED_TOOLS` | (vazio) | lista separada por vírgula de ferramentas liberadas, ex: `Read,Grep,Glob` — restringe ainda mais o que o agente pode fazer |
| `CLAUDEBRIDGE_TIMEOUT_SECONDS` | `300` | timeout por requisição |
| `CLAUDEBRIDGE_SESSION_TTL_MINUTES` | `60` | tempo de inatividade até uma sessão ser descartada da memória |

## Uso

`POST /v1/chat` com corpo JSON:

```json
{ "session_id": "usuario-123", "prompt": "explique o que é uma goroutine" }
```

Resposta:

```json
{ "session_id": "usuario-123", "claude_session_id": "abc-123...", "reply": "..." }
```

- Omitir `session_id` faz uma chamada avulsa (sem histórico).
- Reenviar o mesmo `session_id` continua a mesma conversa via
  `claude --resume <claude_session_id>`.
- Chamadas concorrentes com o mesmo `session_id` são serializadas (uma
  trava por sessão) para não corromper o resume.

```bash
curl -s http://127.0.0.1:8081/v1/chat \
  -H "Authorization: Bearer $CLAUDEBRIDGE_AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"session_id":"teste","prompt":"oi, tudo bem?"}'
```

`GET /healthz` responde `ok` sem autenticação.

## Diferenças em relação ao codex-bridge

- Saída do CLI: um único objeto JSON ao final (`--output-format json`),
  não um stream NDJSON linha a linha como o `codex exec --json`.
- Controle de risco: em vez de sandbox (`read-only`/`workspace-write`), é
  `--permission-mode` + `--allowedTools`. `acceptEdits` ainda pode tentar
  pedir aprovação para comandos de shell fora de edição de arquivo — sem
  terminal interativo isso tende a falhar (não travar), então teste antes
  de usar em produção; se precisar rodar comandos de shell sem prompt,
  use `CLAUDEBRIDGE_ALLOWED_TOOLS` para liberar só o necessário em vez de
  pular para `bypassPermissions`.

## Sobre usar isso com sua assinatura (não API key)

Vale o mesmo raciocínio que discutimos pro codex-bridge: publicar o
código é tranquilo (cada pessoa faz login com a própria conta), o que os
Termos de Uso da Anthropic restringem é compartilhar/expor sua conta para
terceiros usarem através do serviço — então mantenha isso como automação
pessoal (você chamando seu próprio servidor) e evite transformar num
serviço multiusuário hospedado na sua única assinatura.
