# Project Tracker

This file is the single place to understand current product state and next development priorities.

## Scope

Project: `nisix`  
Goal: OpenClaw-aligned assistant runtime in Go with Telegram + WS control plane.

## Current State

### Completed

- Gateway with HTTP + WebSocket (`connect`, `chat.send`, `chat.abort`, `chat.history`, `sessions.list`).
- Telegram adapter with polling, mention policy, allowlist, dedupe, and throttling.
- Persistent sessions + transcripts (`state/sessions.json`, JSONL transcript files).
- Skills V1:
  - workspace discovery from `workspace/skills/*/SKILL.md`
  - explicit invocation + auto-match
  - enabled/allowlist gating
  - `skills.list` WS method
- Bootstrap V2:
  - template seeding without overwrite
  - onboarding state in `.nisix/workspace_state.json`
  - `bootstrap.status` + `bootstrap.complete`
- Realtime profile engine:
  - `/profile list|show|diff|set|append|apply`
  - hybrid proposal flow with TTL/session binding
  - file allowlist and max-size guardrails
  - atomic writes and per-file lock
  - `profile.get` + `profile.update`
- Prompt/runtime quality:
  - per-message context reload
  - deterministic context order: `IDENTITY -> SOUL -> AGENTS -> TOOLS -> USER`
  - no SOUL duplication in project context block
- Model providers:
  - `echo`
  - `openai` / `codex`
  - `ollama`
- MCP tools runtime:
  - load MCP server definitions from `mcp.json`
  - supported transports: `stdio`, `streamable_http`, `sse`
  - register MCP tools in tool registry as `mcp_<server>_<tool>`
  - execute MCP `tools/call` through runtime tool pipeline
  - inspect loaded MCP servers/tool mappings via `/mcp status`, `/mcp tools`, `mcp.status`, and `mcp.tools`
- Telegram runtime: multi-account support via `channels.telegram` + `channels.telegramAccounts` with account-aware outbound routing.

### Decisions Locked

- `profile.updateMode = hybrid`
- `bootstrap.reloadMode = per_message`
- `memory.autoLoadScope = dm_only`
- Skills behavior = explicit + auto-match with allowlist gating

## Priority Backlog

Status legend: `todo`, `in_progress`, `done`, `blocked`.

| ID | Item | Status | Notes |
|---|---|---|---|
| P1 | `tools.catalog` WS method + tool schema introspection | done | Added WS method + catalog metadata/schema from tool registry |
| P2 | Transcript schema v2 (tool calls + metadata + usage) | done | Added v2 fields + backward-compatible history normalization |
| P3 | Multi-account Telegram support | done | Added `channels.telegramAccounts` + account-aware hub/adapters |
| P4 | Typed plugin runtime + sandbox policy | todo | Larger architecture step |
| P5 | Session observability pack (`runId/sessionKey` dashboards/log conventions) | todo | Useful for production operations |

## Key File Map

- Runtime: `internal/agentruntime/runtime.go`
- Bootstrap service: `internal/bootstrap/service.go`
- Profile service: `internal/profile/*`
- Gateway WS: `internal/gateway/ws.go`
- Server integration: `internal/gateway/server.go`
- Model clients: `internal/model/*`
- Config schema/defaults: `internal/config/config.go`
- Daemon wiring: `cmd/nisixd/main.go`

## Update Policy

Update this file when:

1. A feature is added/removed.
2. A decision (default behavior) changes.
3. Backlog priorities are reordered.

