# nisix

Go skeleton for your OpenClaw-inspired assistant architecture.

## Session Handoff Docs

- `docs/SESSION_START.md`: exact checklist to resume development from a new session.
- `docs/PROJECT_TRACKER.md`: current capabilities, locked decisions, and priority backlog.
- `docs/WORK_LOG.md`: ongoing handoff log (what changed, what to do next).

## Implemented core modules

- `gateway`: HTTP + WebSocket control entrypoint.
- `protocol`: `req/res/event` frame types and WS connect params.
- `router`: channel/account/peer to `agentID` + `sessionKey`.
- `agentruntime`: runtime loop, tool dispatch, memory/soul/identity usage.
- `identity`: loads `IDENTITY.md`.
- `soul`: loads `SOUL.md`.
- `tools`: tool registry + sample `time_now` tool.
- `mcp`: loads MCP stdio servers from `mcp.json` and registers remote MCP tools.
- `toolpolicy`: allow/deny policy checks.
- `sessions`: persistent `sessions.json` + transcript JSONL appends.
- `memory`: workspace markdown search service.
- `skills`: workspace skill discovery, parsing, gating, auto-match, explicit invocation.
- `channels`: multi-hub routing + Telegram adapter + stdout fallback.
- `security`: token authenticator.
- `config`: JSON config model + loader.
- `observability`: logger setup.
- `workspace`: ensures bootstrap file layout.

## Run

```bash
cd <repo-root>
cp configs/nisix.local.example.json configs/nisix.local.json
go run ./cmd/nisixd -config configs/nisix.local.json -listen :18789
```

Notes:

- run commands from repo root (directory containing `go.mod`)
- `configs/nisix.local.json` is git-ignored for secrets.
- keep `configs/nisix.example.json` sanitized (no real tokens/keys).

## HTTP API

Health check:

```bash
curl -s http://127.0.0.1:18789/health
```

Send inbound message:

```bash
curl -s http://127.0.0.1:18789/inbound \
  -H 'content-type: application/json' \
  -d '{
    "token": "dev-token",
    "channel": "telegram",
    "accountId": "default",
    "peerId": "123456789",
    "peerType": "direct",
    "userId": "123456789",
    "text": "hello"
  }'
```

## WebSocket API (`/ws`)

Supported methods:

- `connect`
- `health`
- `chat.send`
- `chat.abort`
- `chat.history`
- `sessions.list`
- `skills.list`
- `profile.get`
- `profile.update`
- `bootstrap.status`
- `bootstrap.complete`

Example frame sequence:

```json
{"type":"req","id":"1","method":"connect","params":{"minProtocol":1,"maxProtocol":1,"client":{"id":"cli","version":"0.1.0","platform":"mac"},"auth":{"token":"dev-token"}}}
```

```json
{"type":"req","id":"2","method":"chat.send","params":{"channel":"telegram","accountId":"default","peerId":"123456789","peerType":"direct","userId":"123456789","text":"hi"}}
```

```json
{"type":"req","id":"3","method":"chat.history","params":{"sessionKey":"agent:main:telegram:default:dm:123456789","limit":20}}
```

With filters:

```json
{"type":"req","id":"4","method":"chat.history","params":{"sessionKey":"agent:main:telegram:default:dm:123456789","limit":20,"role":"assistant","from":1772680000000,"to":1772690000000}}
```

Cursor pagination:

```json
{"type":"req","id":"6","method":"chat.history","params":{"sessionKey":"agent:main:telegram:default:dm:123456789","limit":20,"cursor":"0","before":1772690000000}}
```

```json
{"type":"req","id":"5","method":"skills.list","params":{"enabledOnly":false}}
```

```json
{"type":"req","id":"8","method":"profile.get","params":{"file":"IDENTITY.md"}}
```

```json
{"type":"req","id":"9","method":"profile.update","params":{"file":"USER.md","content":"# USER\n\n## Profile\n- **Name:** Thanh\n","mode":"replace","reason":"ui_edit"}}
```

```json
{"type":"req","id":"10","method":"bootstrap.status","params":{}}
```

```json
{"type":"req","id":"11","method":"bootstrap.complete","params":{"removeBootstrap":true}}
```

Abort a run:

```json
{"type":"req","id":"7","method":"chat.abort","params":{"runId":"run-123"}}
```

## Sessions persistence

- Session store: `<session.stateDir>/sessions.json`
- Transcripts: `<session.stateDir>/transcripts/<sessionId>.jsonl`

## Telegram adapter

Configure in `configs/nisix.example.json`:

```json
"channels": {
  "telegram": {
    "accountId": "default",
    "enabled": true,
    "token": "<BOT_TOKEN>",
    "polling": true,
    "botUsername": "",
    "autoDetectBotUsername": true,
    "requireMentionInGroups": true,
    "enableHelpCommands": true,
    "minUserIntervalMs": 700,
    "dedupeWindow": 2048,
    "allowlistMode": "users",
    "allowUsers": ["<ADMIN_TELEGRAM_USER_ID>"],
    "allowChats": []
  },
  "telegramAccounts": [
    {
      "accountId": "work-bot",
      "enabled": true,
      "token": "<SECOND_BOT_TOKEN>",
      "polling": true,
      "botUsername": "",
      "allowlistMode": "users",
      "allowUsers": ["<ADMIN_TELEGRAM_USER_ID>"],
      "allowChats": []
    }
  ]
}
```

Notes:

- `channels.telegram` is the legacy single-account block and is still supported.
- `channels.telegramAccounts` lets you enable multiple Telegram bot accounts in the same runtime.
- each enabled account must have a unique `accountId`.
- inbound updates carry the configured `accountId`, so router bindings can target specific agents.
- outbound replies are routed by `channel + accountId`; if no account match is found, channel fallback is used.
- duplicate `update_id` events are ignored in-memory (`dedupeWindow`).
- per-user inbound throttling is enforced (`minUserIntervalMs`).
- `/start` and `/help` are handled by the adapter when `enableHelpCommands=true`.
- in group chats, messages require `@botUsername` when `requireMentionInGroups=true`.
- if `botUsername` is empty and `autoDetectBotUsername=true`, adapter resolves it from `getMe`.
- optional allowlist policy modes: `off`, `users`, `chats`, `users_or_chats`, `users_and_chats`.

## Model provider

Runtime model config:

```json
"model": {
  "provider": "openai",
  "timeoutSec": 60,
  "openai": {
    "apiKey": "<OPENAI_API_KEY>",
    "baseUrl": "https://api.openai.com/v1",
    "model": "gpt-5-codex"
  },
  "ollama": {
    "baseUrl": "http://127.0.0.1:11434",
    "model": "llama3.2"
  }
}
```

Notes:

- `model.provider=echo` keeps local echo behavior (default).
- `model.provider=openai` (or `codex`) calls OpenAI Responses API.
- `model.provider=ollama` calls local Ollama `/api/generate`.
- if `model.openai.apiKey` is empty, loader falls back to `OPENAI_API_KEY` env var.

Ollama quick start:

```bash
ollama serve
ollama pull llama3.2
```


## MCP tools from mcp.json

Add MCP settings in runtime config:

```json
"mcp": {
  "enabled": true,
  "configFile": "./mcp.json",
  "toolPrefix": "mcp"
}
```

Define MCP servers in `mcp.json` (see `mcp.json.example`):

```json
{
  "mcpServers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "./workspace/main"],
      "env": {},
      "cwd": "."
    }
  }
}
```

At startup, nisix discovers MCP tools and registers them as local tool names using the pattern `mcp_<server>_<tool>`.
If `tools.allow` is non-empty, include the generated MCP tool names there as well.
## Skills V1

Workspace folder layout:

```text
workspace/main/
  skills/
    <skill-name>/
      SKILL.md
```

`SKILL.md` format:

```md
---
name: architecture
description: architecture planning and implementation steps
---

Use concise implementation steps.
```

Config flags:

```json
"skills": {
  "enabled": true,
  "autoMatch": true,
  "maxInjected": 1,
  "allowlist": [],
  "maxBodyChars": 4000,
  "entries": {
    "architecture": { "enabled": true }
  }
}
```

Explicit invocation in message:

- `/skill architecture`
- `!skill architecture`

Selection behavior:

- explicit skill call wins over auto-match
- if no explicit call, best match from message text is selected when `autoMatch=true`
- selected skill content is injected into runtime context as:
  - `## Skill: <name>`
  - `<skill body>`

## Bootstrap V2 + Profile Runtime

Config flags:

```json
"workspace": {
  "bootstrapFromTemplates": true,
  "templateDir": "./workspace/templates"
},
"bootstrap": {
  "reloadMode": "per_message"
},
"profile": {
  "updateMode": "hybrid",
  "autoDetectEnabled": true,
  "allowedFiles": ["IDENTITY.md", "SOUL.md", "USER.md", "TOOLS.md", "AGENTS.md", "MEMORY.md"],
  "maxFileBytes": 262144
},
"memory": {
  "enabled": true,
  "autoLoadScope": "dm_only"
}
```

Behavior:

- workspace bootstrap seeds missing files from `workspace/templates/*` and never overwrites existing files
- onboarding state stored at `<workspace>/.nisix/workspace_state.json`
- runtime reloads project context every message (`AGENTS.md`, `SOUL.md`, `USER.md`, `TOOLS.md`, optional `BOOTSTRAP.md`)
- prompt injection order for model requests is deterministic: `IDENTITY -> SOUL -> AGENTS -> TOOLS -> USER` (then skills, then memory hits)
- memory auto-load is skipped for group/channel chats when `memory.autoLoadScope=dm_only`
- profile update flow supports explicit commands and hybrid proposal flow

Chat commands:

- `/profile list`
- `/profile show <FILE>`
- `/profile diff <FILE>` (with content, or without content to diff latest proposal for that file)
- `/profile set <FILE>` then put content (same message line or next lines)
- `/profile append <FILE> ...`
- `/profile apply <proposal_id>`
- `/onboard status`
- `/onboard done`

Hybrid proposal flow:

- high-confidence intent creates proposal only (no file write)
- bot replies with proposal id
- user confirms with `/profile apply <proposal_id>`

Security notes:

- only files in `profile.allowedFiles` are writable
- max payload enforced by `profile.maxFileBytes`
- writes are atomic (`tmp + rename`) and serialized per file path

Migration note:

- existing workspaces keep current files
- bootstrap only seeds missing files
- `BOOTSTRAP.md` is one-time and removed after onboarding complete

## Next build steps

1. Add `chat.history` filter by `eventType` for efficient UI retrieval.
2. Add typed plugin/skill runtime + sandboxed tool execution.
3. Add session observability pack (`runId/sessionKey` logging conventions and dashboards).
4. Migrate model-output tool bridge to provider-native structured function calling.





