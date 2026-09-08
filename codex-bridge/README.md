# codex-bridge

Servidor HTTP em Go que expõe o CLI `codex` (autenticado com seu login do
ChatGPT, sem API key) como um endpoint de chat. Roda igual dentro do WSL
ou numa VPS — o que muda é só a configuração via variáveis de ambiente.

Não é um proxy simples: `codex exec` é um agente que pode executar comandos
reais no sistema. Por isso o padrão é o sandbox mais restrito (`read-only`)
e autenticação obrigatória fora de `localhost`.

## Pré-requisitos

1. `codex` instalado e no PATH de quem for rodar o servidor.
2. Login feito uma vez, interativamente: `codex login` (usa o seu plano
   ChatGPT, fica em cache em `~/.codex/auth.json`; o servidor não pede
   login de novo).
3. Go 1.22+ para compilar.

## Build

```bash
cd codex-bridge
go build -o codex-bridge .
```

## Rodar no WSL (uso local, a partir do Windows)

```bash
./codex-bridge
```

Sem nenhuma variável definida, ele sobe em `127.0.0.1:8080`, sandbox
`read-only`, sem exigir token (seguro porque só aceita conexão local — o
Windows enxerga isso automaticamente em `localhost:8080`).

## Rodar numa VPS (exposto na rede)

```bash
export CODEXBRIDGE_LISTEN_ADDR=0.0.0.0:8080
export CODEXBRIDGE_AUTH_TOKEN="um-token-longo-e-aleatorio"
export CODEXBRIDGE_SANDBOX=workspace-write   # só se precisar que ele escreva arquivos
./codex-bridge
```

Se `CODEXBRIDGE_LISTEN_ADDR` não for loopback, o servidor **recusa subir**
sem `CODEXBRIDGE_AUTH_TOKEN` definido — evita esquecer autenticação em
produção. Numa VPS real, coloque isso atrás de um reverse proxy com TLS
(Caddy/nginx) e um firewall restringindo os IPs de origem; o token sozinho
não substitui isso na internet aberta.

## Variáveis de ambiente

| Variável | Padrão | Descrição |
|---|---|---|
| `CODEXBRIDGE_LISTEN_ADDR` | `127.0.0.1:8080` | endereço:porta do listener |
| `CODEXBRIDGE_AUTH_TOKEN` | (vazio) | token Bearer exigido no header `Authorization`; obrigatório se o bind não for loopback |
| `CODEXBRIDGE_CODEX_BIN` | `codex` | caminho/nome do binário do CLI |
| `CODEXBRIDGE_WORKDIR` | `.` | diretório de trabalho passado ao codex (`-C`) em sessões novas |
| `CODEXBRIDGE_SANDBOX` | `read-only` | `read-only` \| `workspace-write` \| `danger-full-access` |
| `CODEXBRIDGE_APPROVAL_POLICY` | `never` | política de aprovação do codex (precisa ser `never` para não travar sem terminal interativo) |
| `CODEXBRIDGE_TIMEOUT_SECONDS` | `300` | timeout por requisição |
| `CODEXBRIDGE_SESSION_TTL_MINUTES` | `60` | tempo de inatividade até uma sessão ser descartada da memória |

## Uso

`POST /v1/chat` com corpo JSON:

```json
{ "session_id": "usuario-123", "prompt": "explique o que é um goroutine" }
```

Resposta:

```json
{ "session_id": "usuario-123", "thread_id": "019bd4...", "reply": "..." }
```

- Omitir `session_id` faz uma chamada avulsa (sem histórico).
- Reenviar o mesmo `session_id` continua a mesma conversa via
  `codex exec resume <thread_id>` — o codex mantém o contexto.
- Chamadas concorrentes com o mesmo `session_id` são serializadas
  automaticamente (uma trava por sessão) para não corromper o resume.

Exemplo com `curl`, incluindo token:

```bash
curl -s http://127.0.0.1:8080/v1/chat \
  -H "Authorization: Bearer $CODEXBRIDGE_AUTH_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"session_id":"teste","prompt":"oi, tudo bem?"}'
```

`GET /healthz` responde `ok` sem autenticação, para health check de infra.

## Limitações conhecidas

- O parsing dos eventos `--json` do codex (`thread.started`, `item.completed`
  com `agent_message`) é baseado na versão atual documentada do CLI; se uma
  atualização do `codex` mudar esse formato, o servidor vai retornar erro
  "codex produced no reply" — rode o comando manualmente
  (`codex exec --json "teste"`) pra conferir o formato antes de abrir issue.
- Não há streaming: a resposta só volta depois que o `codex exec` termina
  o turno inteiro.
