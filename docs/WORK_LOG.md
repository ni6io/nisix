# Work Log

Use this as the running handoff log between sessions.

## Entry Template

```md
## YYYY-MM-DD HH:MM (timezone)

### Context Loaded
- Branch:
- Last commit:
- Tracker status reviewed: yes/no

### Changes Made
- ...

### Validation
- `go test ./...`: pass/fail
- `go vet ./...`: pass/fail
- Manual checks:

### Risks / Follow-up
- ...

### Next Session First Step
- ...
```

---

## 2026-03-05 16:45 (Asia/Ho_Chi_Minh)

### Context Loaded
- Branch: `main`
- Last major milestone: Bootstrap V2 + Profile runtime + Skills V1 + Ollama provider
- Tracker status reviewed: yes

### Changes Made
- Added session continuity docs:
  - `docs/SESSION_START.md`
  - `docs/PROJECT_TRACKER.md`
  - `docs/WORK_LOG.md`
- Linked handoff docs from README.

### Validation
- Docs-only change (no runtime logic changed).

### Risks / Follow-up
- Keep this log updated every coding session; otherwise handoff quality degrades quickly.

### Next Session First Step
- Open `docs/SESSION_START.md` and run the checklist.

## 2026-03-05 17:41 (Asia/Ho_Chi_Minh)

### Context Loaded
- Branch: `main`
- Tracker status reviewed: yes

### Changes Made
- Implemented `tools.catalog` over WS:
  - Added `tools.catalog` to connect feature methods.
  - Added WS handling to return `{"tools": [...]}` payload.
- Added tool schema introspection in `internal/tools`:
  - Introduced tool `Metadata` with `inputSchema`/`outputSchema`.
  - Added `Registry.Catalog()` with deterministic sorting and default object input schema.
  - Added metadata for built-in `time_now` tool.
- Wired gateway to serve catalog from the shared tool registry.
- Added test coverage:
  - WS integration assertion for `tools.catalog` handshake + payload.
  - Unit tests for `Registry.Catalog()` behavior.

### Validation
- `go test ./...`: pass
- `go vet ./...`: pass

### Risks / Follow-up
- Current tool execution remains explicit (`!tool <name>`); model-driven auto tool-calling is not implemented yet.

### Next Session First Step
- Implement structured tool invocation flow (model/tool-call protocol) to move beyond manual `!tool` command path.

## 2026-03-05 17:52 (Asia/Ho_Chi_Minh)

### Context Loaded
- Branch: `codex/tools-catalog-introspection`
- Tracker status reviewed: yes

### Changes Made
- Fixed runtime behavior where model output could stop at a tool "plan" (e.g. `SERVER_TIME_NOW: time_now()`) without executing the tool.
- Added a model-output tool-call bridge in runtime:
  - Parse strict tool-call lines from generated text.
  - Execute matched registered tool via existing policy guardrails.
  - Return concrete final output for `time_now` as `Server time now: <RFC3339>`.
  - Emit tool event before final response when execution succeeds.
- Added tests in `internal/agentruntime/runtime_model_test.go`:
  - executes tool call emitted by model output.
  - enforces deny policy for model-emitted tool call.

### Validation
- `go test ./...`: pass
- `go vet ./...`: pass

### Risks / Follow-up
- Current parser intentionally supports one simple call pattern per line; full multi-tool/function-call protocol remains a future step.

### Next Session First Step
- Migrate this bridge to structured model function-calling payloads (provider-native) and support typed arguments.

## 2026-03-05 18:03 (Asia/Ho_Chi_Minh)

### Context Loaded
- Branch: `main`
- Tracker status reviewed: yes

### Changes Made
- Added chat commands for listing capabilities directly in runtime:
  - `/skills list` and `/skill list`
  - `/tools list` and `/tool list`
- Command behavior:
  - Skills list loads current workspace skills and shows enabled/disabled status with reason when blocked.
  - Tools list shows registered tools and whether each tool is allowed/blocked by current tool policy.
  - Both command paths bypass model generation.
- Added tests:
  - `TestRuntimeSkillsListCommand`
  - `TestRuntimeToolsListCommand`

### Validation
- `go test ./...`: pass
- `go vet ./...`: pass

### Risks / Follow-up
- Output is plain text for now; if UI needs structured list in chat flow, add a typed event or JSON mode.

### Next Session First Step
- Add `/skills show <name>` or `/tools show <name>` command to inspect details/schema quickly from chat.

## 2026-03-05 18:13 (Asia/Ho_Chi_Minh)

### Context Loaded
- Branch: `main`
- Tracker status reviewed: yes

### Changes Made
- Implemented transcript schema v2 in session storage:
  - Extended transcript record fields with `schemaVersion`, `eventType`, `runId`, `kind`, `provider`, `toolCall`, `usage`, and `metadata`.
  - Added `AppendWithOptions(...)` to preserve old `Append(...)` API while supporting richer writes.
  - Added record normalization on history reads for backward-compatible handling of legacy v1 rows.
- Wired gateway/runtime to write richer transcript rows:
  - User input now stores metadata (`channel/account/peer/user/thread`) and run context.
  - Assistant events now store event-kind mapping (`message`, `message_chunk`, `tool_call`, `event`) with run/provider/tool metadata.
  - Tool events now carry structured `toolCall` details from runtime.
- Extended WS agent event payload to include `provider`, `toolCall`, and `usage`.
- Added/updated tests:
  - Session manager tests for v2 write + legacy normalization.
  - Gateway integration test asserting `tool_call` row with `runId` appears in `chat.history`.

### Validation
- `go test ./...`: pass
- `go vet ./...`: pass

### Risks / Follow-up
- Streaming/block events are now persisted as transcript rows (`eventType=message_chunk`), which increases transcript volume for long outputs.

### Next Session First Step
- Add `chat.history` filter by `eventType` to make retrieval of only final/tool rows efficient for UIs.

## 2026-03-05 21:47 (Asia/Ho_Chi_Minh)

### Context Loaded
- Branch: `main`
- Last commit: `753efcb`
- Tracker status reviewed: yes

### Changes Made
- Implemented P3 multi-account Telegram runtime support:
  - Extended config schema with `channels.telegramAccounts[]` and per-account `accountId`.
  - Added Telegram config defaults/validation for both legacy single block and multi-account list.
  - Added duplicate `accountId` detection and required-token validation for enabled Telegram accounts.
- Updated Telegram channel routing/runtime:
  - `internal/channels.MultiHub` now supports account-aware sender mapping via `RegisterAccount(channel, accountId, sender)`.
  - `internal/channels.TelegramAdapter` now carries configured `accountId` and emits inbound messages with that account id (no hardcoded `default`).
  - `cmd/nisixd/main.go` now boots multiple Telegram adapters (legacy `channels.telegram` + `channels.telegramAccounts`) and starts polling per enabled account.
- Added/updated tests:
  - `internal/channels/hub_test.go` for channel+account dispatch behavior.
  - `internal/channels/telegram_test.go` for accountId normalization/defaulting.
  - `internal/config/config_test.go` for telegram account defaults and duplicate account validation.
- Updated docs/config samples:
  - `configs/nisix.example.json` and `configs/nisix.local.example.json` include `accountId` and `telegramAccounts` examples.
  - `README.md` Telegram section documents multi-account config and account-aware routing.
  - `docs/PROJECT_TRACKER.md` marks P3 as done and records feature in current state.

### Validation
- `go test ./...`: pass
- `go vet ./...`: pass
- Manual checks:
  - Verified default + account-specific hub routing behavior via unit tests.

### Risks / Follow-up
- When both `channels.telegram` and `channels.telegramAccounts` are enabled, account IDs must be unique; startup now fails fast on duplicates.

### Next Session First Step
- Implement P4 foundation: define typed plugin runtime interfaces and initial sandbox policy contract.

## 2026-03-05 22:12 (Asia/Ho_Chi_Minh)

### Context Loaded
- Branch: `main`
- Last commit: `753efcb`
- Tracker status reviewed: yes

### Changes Made
- Added MCP tool runtime support with `mcp.json` definitions:
  - New package `internal/mcp` with stdio MCP client (initialize, tools/list, tools/call).
  - Added loader to read `mcp.json`, start configured servers, discover tools, and register them into tool registry.
  - MCP tools are exposed as local tool names: `mcp_<server>_<tool>`.
- Wired daemon startup to load MCP tools from config:
  - Added `mcp` config section (`enabled`, `configFile`, `toolPrefix`) with defaults.
  - Startup now loads MCP tools before runtime initialization and closes MCP clients on shutdown.
- Added tests:
  - `internal/mcp/loader_test.go` with helper MCP stdio server process.
  - `internal/config/config_test.go` assertions for new MCP defaults.
- Updated docs/config examples:
  - `README.md` MCP section.
  - `configs/nisix.example.json` and `configs/nisix.local.example.json` with `mcp` config block.
  - Added `mcp.json.example` and ignored local `mcp.json` in `.gitignore`.
  - Updated tracker completed list with MCP runtime item.

### Validation
- `go test ./...`: pass
- `go vet ./...`: pass
- Manual checks:
  - Verified MCP package unit test executes real stdio JSON-RPC flow (initialize/list/call).

### Risks / Follow-up
- Current MCP implementation supports stdio transport only (no SSE/streaming transport yet).
- Tool name sanitization may produce suffixes (`_2`, `_3`) on collisions; document naming if UI needs stable explicit mapping.

### Next Session First Step
- Add `mcp.status`/`mcp.tools` debug command in chat/WS to inspect loaded MCP servers and tool mappings at runtime.

