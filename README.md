# agent-bridge

Expõe o **Codex CLI** ou o **Claude Code** — autenticados com a sua assinatura (ChatGPT plan / Claude Pro-Max), **sem API key** — como um servidor HTTP local.

Um único binário que detecta o que você tem instalado, te ajuda no login e sobe já configurado. Substitui os protótipos separados `codex-bridge` e `claude-bridge`.

> ⚠️ **Uso pessoal.** Isto é pensado para automação sua (você chamando o seu próprio servidor com a sua própria conta). Transformar em serviço multiusuário sobre uma única assinatura viola os Termos de Uso da OpenAI/Anthropic.

---

## Requisitos

| Dependência | Versão | Observação |
|---|---|---|
| **Go** | 1.22+ | Usa o roteamento por método/path do `net/http`, que só existe a partir do 1.22 |
| **Codex CLI** e/ou **Claude Code** | — | Instale ao menos um dos dois (veja abaixo) |
| **Node/npm** | — | Só necessário para o Codex CLI |

---

## Instalação (WSL / Linux)

### 1. Entrar na pasta do projeto

Se o projeto está no Windows, o WSL enxerga o disco `C:` em `/mnt/c/`:

```bash
cd "/mnt/c/Projetos/nivus sei la/agent-bridge"
```

> **Recomendado:** compilar em `/mnt/c` funciona, mas é mais lento e às vezes dá dor de cabeça com permissão. Para mais tranquilidade, copie a pasta para dentro do WSL:
> ```bash
> cp -r "/mnt/c/Projetos/nivus sei la/agent-bridge" ~/agent-bridge && cd ~/agent-bridge
> ```

### 2. Instalar o Go (1.22+)

```bash
go version   # confere se já tem, e a versão
```

Se não tiver, ou for menor que 1.22, instale a versão oficial (mais confiável que o `apt`, que costuma vir desatualizado). Veja o arquivo mais recente em <https://go.dev/dl/> e ajuste o nome abaixo:

```bash
wget https://go.dev/dl/go1.23.4.linux-amd64.tar.gz -O /tmp/go.tar.gz
sudo rm -rf /usr/local/go && sudo tar -C /usr/local -xzf /tmp/go.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
go version
```

### 3. Instalar o Codex CLI e/ou o Claude Code

**Codex CLI** (precisa de Node/npm):

```bash
node -v || (sudo apt update && sudo apt install -y nodejs npm)
npm install -g @openai/codex
```

**Claude Code** (instalador nativo, não precisa de Node):

```bash
curl -fsSL https://claude.ai/install.sh | bash
```

Confirme que estão no PATH:

```bash
codex --version
claude --version
```

### 4. Compilar

```bash
go build -o agent-bridge .
```

Opcional — deixar disponível em qualquer pasta:

```bash
sudo cp agent-bridge /usr/local/bin/
```

---

## Uso do dia a dia

### Primeira vez — assistente interativo

```bash
./agent-bridge
```

O assistente vai:

1. Detectar se `codex` e/ou `claude` estão no PATH (e sugerir o comando de instalação se nenhum estiver — ele **não instala nada sozinho**).
2. Perguntar qual usar, se os dois estiverem instalados.
3. Conferir se você já fez login (`codex login`, ou `/login` dentro do `claude`) e oferecer abrir o CLI para você logar ali mesmo.
4. Perguntar se vai rodar só localmente (WSL/localhost) ou numa VPS exposta — no segundo caso, gera um token de autenticação automaticamente e avisa para colocar atrás de TLS/firewall.
5. Perguntar se o agente pode escrever/editar arquivos ou deve ficar somente leitura.
6. Salvar tudo em `~/.agent-bridge/config.json` e subir o servidor.

### Da segunda vez em diante

```bash
./agent-bridge          # carrega a config salva e sobe direto, sem perguntar nada
./agent-bridge doctor   # só checa se está tudo ok, sem subir o servidor
```

---

## Comandos

| Comando | Quando usar |
|---|---|
| `agent-bridge` | Uso normal: assistente na primeira vez, direto para produção depois |
| `agent-bridge init` | Reconfigurar do zero (trocar engine, porta, permissões etc.) |
| `agent-bridge serve` | Inicia sem perguntar nada — falha se não houver config salva; use em systemd/produção |
| `agent-bridge doctor` | Checa instalação, login e configuração sem subir o servidor |

---

## Configuração avançada (variáveis de ambiente)

A configuração salva pelo assistente pode ser sobrescrita por variável de ambiente, sem editar o arquivo — útil em container/systemd:

| Variável | Efeito |
|---|---|
| `AGENTBRIDGE_ENGINE` | `codex` ou `claude` |
| `AGENTBRIDGE_LISTEN_ADDR` | `endereço:porta` |
| `AGENTBRIDGE_AUTH_TOKEN` | Token Bearer exigido no header `Authorization` |
| `AGENTBRIDGE_WORKDIR` | Diretório de trabalho do agente |
| `AGENTBRIDGE_CODEX_BIN` / `AGENTBRIDGE_CODEX_SANDBOX` / `AGENTBRIDGE_CODEX_APPROVAL_POLICY` | Ajustes do engine codex |
| `AGENTBRIDGE_CLAUDE_BIN` / `AGENTBRIDGE_CLAUDE_PERMISSION_MODE` / `AGENTBRIDGE_CLAUDE_ALLOWED_TOOLS` (lista separada por vírgula) | Ajustes do engine claude |
| `AGENTBRIDGE_TIMEOUT_SECONDS` | Timeout por requisição (padrão `300`) |
| `AGENTBRIDGE_SESSION_TTL_MINUTES` | Expiração de sessão inativa (padrão `60`) |

> Se `AGENTBRIDGE_LISTEN_ADDR` (ou o valor salvo) **não** for loopback, um token é **obrigatório** — o programa recusa subir sem isso.

---

## Uso da API

A porta/URL da API é exatamente a que aparece no banner quando o servidor sobe (e em `agent-bridge doctor`) — é para lá que qualquer cliente deve apontar.

A API é **assíncrona**: um turno do codex/claude pode levar minutos, e a maioria dos clientes HTTP (e proxies) desiste antes disso. Por isso `POST /v1/chat` responde na hora com um `job_id`, e você busca o resultado depois com `GET /v1/jobs/{job_id}`.

### 1. Pedir

```bash
curl -s http://127.0.0.1:8080/v1/chat \
  -H "Content-Type: application/json" \
  -d '{"session_id":"teste","prompt":"oi, tudo bem?"}'
```

```json
{ "job_id": "job_a1b2c3d4e5f6", "session_id": "teste", "status": "pending", "created_at": "...", "updated_at": "..." }
```

### 2. Buscar a resposta

Repita até `status` virar `"done"` (ou `"error"` — veja o campo `error`):

```bash
curl -s http://127.0.0.1:8080/v1/jobs/job_a1b2c3d4e5f6
```

```json
{ "job_id": "job_a1b2c3d4e5f6", "session_id": "teste", "status": "done",
  "reply": "...", "engine_session_id": "abc123...", "created_at": "...", "updated_at": "..." }
```

> Se você configurou VPS/token, adicione `-H "Authorization: Bearer <token>"` nos dois comandos.

### Detalhes

- Omitir `session_id` no passo 1 faz uma chamada avulsa (sem histórico).
- Reenviar o mesmo `session_id` continua a mesma conversa (via `codex exec resume` ou `claude --resume`, dependendo do engine ativo).
- Jobs terminados (`done`/`error`) somem depois de `AGENTBRIDGE_SESSION_TTL_MINUTES` sem serem consultados — não dá para buscar um `job_id` muito antigo para sempre.

### Endpoints sem autenticação

| Endpoint | Uso |
|---|---|
| `GET /healthz` | Health check simples |
| `GET /v1/info` | Retorna o engine ativo e um lembrete do formato da API — útil para confirmar que é nessa porta que se deve chamar |

---

## Limitações conhecidas

- **Parsing do `--json`:** segue o formato documentado hoje de cada CLI. Se uma atualização mudar o schema, o erro aparece como `"produced no reply"` ou `"parsing json output"`. Rode o comando manualmente para comparar antes de abrir issue:
  ```bash
  codex exec --json "teste"
  claude --print --output-format json "teste"
  ```
- **Sem streaming:** a resposta só volta depois que o turno inteiro termina.
- **Uso pessoal apenas:** ver aviso no topo.

---

## Sobre os protótipos antigos

As pastas `codex-bridge/` e `claude-bridge/` continuam no disco como protótipos separados, mas o `agent-bridge` cobre os dois casos num único binário. Pode apagá-las se não for mais usá-las separadamente.
