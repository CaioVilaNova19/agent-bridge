# agent-bridge

Ferramenta única que expõe o **Codex CLI** ou o **Claude Code** (ambos
autenticados com sua assinatura — ChatGPT plan / Claude Pro-Max —, sem API
key) como um servidor HTTP local. Substitui os protótipos separados
`codex-bridge` e `claude-bridge`: agora é um único binário que detecta o
que você tem instalado, ajuda no login e sobe já configurado.

## Build

```bash
cd agent-bridge
go build -o agent-bridge .
```

## Uso do dia a dia

```bash
./agent-bridge
```

Na primeira vez, ele roda um assistente interativo:

1. Detecta se `codex` e/ou `claude` estão no PATH (e te dá o comando de
   instalação se nenhum estiver — ele não instala nada sozinho).
2. Se os dois estiverem instalados, pergunta qual usar.
3. Confere se você já fez login (`codex login`, ou `/login` dentro do
   `claude`) e oferece abrir o CLI pra você logar ali mesmo se não tiver.
4. Pergunta se isso vai rodar só localmente (WSL/localhost) ou numa VPS
   exposta — no segundo caso já gera um token de autenticação sozinho e
   avisa pra colocar atrás de TLS/firewall.
5. Pergunta se o agente pode escrever/editar arquivos ou deve ficar
   somente leitura.
6. Salva tudo em `~/.agent-bridge/config.json` e sobe o servidor.

Da segunda vez em diante, `./agent-bridge` pula direto pro passo 6 —
carrega a configuração salva e já sobe o servidor, sem perguntar nada de
novo.

## Comandos

| Comando | Quando usar |
|---|---|
| `agent-bridge` | uso normal: assistente na primeira vez, direto pra produção depois |
| `agent-bridge init` | reconfigurar do zero (trocar engine, porta, permissões etc.) |
| `agent-bridge serve` | inicia sem perguntar nada — falha se não houver config salva; use em systemd/produção |
| `agent-bridge doctor` | checa instalação, login e configuração sem subir o servidor |

## Configuração avançada (variáveis de ambiente)

A configuração salva pelo assistente pode ser sobrescrita por variável de
ambiente, sem editar o arquivo — útil em container/systemd:

| Variável | Efeito |
|---|---|
| `AGENTBRIDGE_ENGINE` | `codex` ou `claude` |
| `AGENTBRIDGE_LISTEN_ADDR` | endereço:porta |
| `AGENTBRIDGE_AUTH_TOKEN` | token Bearer exigido no header `Authorization` |
| `AGENTBRIDGE_WORKDIR` | diretório de trabalho do agente |
| `AGENTBRIDGE_CODEX_BIN` / `AGENTBRIDGE_CODEX_SANDBOX` / `AGENTBRIDGE_CODEX_APPROVAL_POLICY` | ajustes do engine codex |
| `AGENTBRIDGE_CLAUDE_BIN` / `AGENTBRIDGE_CLAUDE_PERMISSION_MODE` / `AGENTBRIDGE_CLAUDE_ALLOWED_TOOLS` (lista separada por vírgula) | ajustes do engine claude |
| `AGENTBRIDGE_TIMEOUT_SECONDS` | timeout por requisição (padrão 300) |
| `AGENTBRIDGE_SESSION_TTL_MINUTES` | expiração de sessão inativa (padrão 60) |

Se `AGENTBRIDGE_LISTEN_ADDR` (ou o valor salvo) não for loopback, é
obrigatório ter um token — o programa recusa subir sem isso.

## Uso da API

**A porta/URL da API é exatamente o que aparece no banner quando o
servidor sobe** (e em `agent-bridge doctor`) — é nisso que qualquer coisa
que for chamar o agent-bridge deve apontar.

A API é **assíncrona**: um turno do codex/claude pode levar minutos, e a
maioria dos clientes HTTP (e proxies) desiste bem antes disso — então em
vez de deixar a conexão aberta esperando, `POST /v1/chat` responde na
hora com um `job_id`, e você consulta o resultado depois com
`GET /v1/jobs/{job_id}`.

**1) Pedir:**

```bash
curl -s http://127.0.0.1:8080/v1/chat \
  -H "Content-Type: application/json" \
  -d '{"session_id":"teste","prompt":"oi, tudo bem?"}'
```

```json
{ "job_id": "job_a1b2c3d4e5f6", "session_id": "teste", "status": "pending", "created_at": "...", "updated_at": "..." }
```

**2) Buscar a resposta** (repita até `status` virar `"done"` — ou
`"error"`, veja o campo `error`):

```bash
curl -s http://127.0.0.1:8080/v1/jobs/job_a1b2c3d4e5f6
```

```json
{ "job_id": "job_a1b2c3d4e5f6", "session_id": "teste", "status": "done",
  "reply": "...", "engine_session_id": "abc123...", "created_at": "...", "updated_at": "..." }
```

- Omitir `session_id` no passo 1 faz uma chamada avulsa (sem histórico).
- Reenviar o mesmo `session_id` continua a mesma conversa (via
  `codex exec resume` ou `claude --resume`, dependendo do engine ativo).
- Jobs terminados (`done`/`error`) somem depois de
  `AGENTBRIDGE_SESSION_TTL_MINUTES` sem serem consultados — não dá pra
  buscar um `job_id` muito antigo pra sempre.
- `GET /healthz` — health check simples, sem autenticação.
- `GET /v1/info` — retorna o engine ativo e um lembrete do formato da API,
  sem autenticação — útil pra confirmar rapidamente que é nessa porta que
  se deve chamar.

## O que ficou dos protótipos antigos

As pastas `codex-bridge/` e `claude-bridge/` continuam no disco como
protótipos separados, mas o `agent-bridge` cobre os dois casos num único
binário — pode apagar as pastas antigas se não for mais usá-las
separadamente.

## Limitações conhecidas

- O parsing do `--json` de cada CLI segue o formato documentado hoje; se
  uma atualização mudar o schema, o erro vai aparecer como "produced no
  reply" ou "parsing json output" — rode o comando manualmente
  (`codex exec --json "teste"` / `claude --print --output-format json
  "teste"`) pra comparar antes de abrir issue.
- Sem streaming: a resposta só volta depois que o turno inteiro termina.
- O mesmo raciocínio sobre Termos de Uso que já discutimos vale aqui: isso
  é pensado para automação **pessoal** (você chamando seu próprio
  servidor com sua própria conta). Transformar isso num serviço
  multiusuário hospedado numa única assinatura é o cenário que os Termos
  da OpenAI/Anthropic proíbem.
