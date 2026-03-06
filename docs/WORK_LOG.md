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

## 2026-03-06 14:44 (Asia/Ho_Chi_Minh)

### Context Loaded
- Branch: `main`
- Last commit: `c3d7c8d`
- Tracker status reviewed: yes

### Changes Made
- Reworked transcript summarization into persisted rolling session context:
  - `internal/sessions.Manager` now maintains recent context window and rolling conversation summary incrementally during transcript appends.
  - Context state is stored in `state/sessions.json` alongside each session entry, so normal turns no longer need to rescan transcript files to build model context.
  - Legacy sessions or changed context budgets trigger one-time rebuild from transcript, then continue incrementally.
- Added configurable context budget settings:
  - `session.contextHistoryLimit`
  - `session.contextSummaryMaxChars`
  - `session.contextSummaryLineChars`
- Simplified gateway context loading:
  - `internal/gateway/server.go` now asks `sessions.ModelContext(...)` for history + summary instead of rebuilding it locally.
- Added regression coverage:
  - `internal/sessions/manager_test.go` covers rolling summary persistence and legacy transcript rebuild.
  - `internal/gateway/server_history_test.go` now verifies summary behavior with custom context budget.
  - `internal/config/config_test.go` covers new session-context defaults.
- Updated docs/config examples:
  - `README.md`
  - `configs/nisix.example.json`
  - `configs/nisix.local.example.json`
  - `docs/PROJECT_TRACKER.md`

### Validation
- `go test ./internal/sessions ./internal/gateway ./internal/config ./cmd/nisixd`: pass
- `go test ./...`: pass
- Manual checks:
  - Not run

### Risks / Follow-up
- Rolling summary is now persistent and cheap to maintain, but it is still string-based compression rather than semantic/task-aware agent memory.
- Very old sessions rebuilt from transcript once on first access after deploy; large transcripts may still incur a one-time catch-up cost.

### Next Session First Step
- If agent continuity still needs improvement, add semantic session-state fields (goal, plan, blockers, key tool results) instead of relying only on compressed chat text.

## 2026-03-06 14:26 (Asia/Ho_Chi_Minh)

### Context Loaded
- Branch: `main`
- Last commit: `d924810`
- Tracker status reviewed: yes

### Changes Made
- Fixed chat context loss by threading recent conversation history back into model requests:
  - `gateway` now reads transcript messages before appending the current user turn.
  - `runtime` forwards filtered `user`/`assistant` history to the model layer.
  - `openai/codex` requests now send prior turns as separate chat messages.
  - `ollama` requests now include a structured conversation-history block in the prompt.
- Added transcript summarization for long sessions:
  - `gateway` keeps the latest 24 transcript messages as raw history.
  - Older transcript messages are compressed into a deterministic `Conversation summary` block and injected into the model system prompt.
  - Summary generation skips chunk/tool events and bounds excerpt count/length to control prompt growth.
- Added regression coverage:
  - `internal/gateway/server_history_test.go` ensures transcript history reaches runtime, excludes chunks/tool events, and summarizes older turns.
  - `internal/agentruntime/runtime_model_test.go` ensures runtime forwards both history and conversation summary.
  - `internal/model/*_test.go` ensures provider payloads include both history and summary context.
- Updated tracker note to reflect history reinjection plus transcript summarization in model calls.

### Validation
- `go test ./internal/model ./internal/agentruntime ./internal/gateway`: pass
- `go test ./...`: pass
- Manual checks:
  - Not run

### Risks / Follow-up
- History window and summary bounds are still fixed constants; if operators need tuning per deployment, move them into config.
- Ollama still uses `/api/generate`; history is serialized into the prompt rather than using provider-native chat messages.

### Next Session First Step
- If context quality is still weak in long chats, add configurable history/summary limits or persist rolling summaries instead of rebuilding them from transcript each turn.

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

## 2026-03-06 12:05 (Asia/Ho_Chi_Minh)

### Context Loaded
- Branch: `main`
- Last commit: `55e85ff`
- Tracker status reviewed: yes

### Changes Made
- Added MCP runtime inspection surfaces for chat and WS:
  - Chat commands: `/mcp status` and `/mcp tools`.
  - WS methods: `mcp.status` and `mcp.tools`.
  - WS connect feature list now advertises both MCP methods.
- Extended `internal/mcp.Manager` with read-only snapshots:
  - status snapshot includes config path, tool prefix, registered tool count, and per-server transport/tool counts.
  - tool mappings include local tool name to remote `server.tool` mapping.
- Wired MCP inspector into daemon/runtime/gateway without changing constructor fanout:
  - added setter-based injection from `cmd/nisixd/main.go`.
- Added tests:
  - runtime command tests for `/mcp status` and `/mcp tools`.
  - WS integration assertions for `mcp.status` and `mcp.tools`.
  - MCP manager test coverage for status/tool snapshots.
- Updated tracker current-state notes for MCP inspection support.

### Validation
- `go test ./internal/mcp ./internal/agentruntime ./internal/gateway`: pass

### Risks / Follow-up
- `mcp.status` currently reports startup-loaded state, not active health probes; if live liveness is needed, add periodic ping/heartbeat semantics.

### Next Session First Step
- Start P4 foundation: define typed plugin runtime interfaces and sandbox policy contract, reusing MCP/tool metadata shapes where possible.


## 2026-03-05 22:01 (Asia/Ho_Chi_Minh)

### Context Loaded
- Branch: `main`
- Tracker status reviewed: yes

### Changes Made
- Extended MCP runtime transport support in `internal/mcp`:
  - Added transport selection per server config: `stdio`, `streamable_http`, `sse`.
  - Kept existing stdio behavior.
  - Added HTTP JSON-RPC request flow for `streamable_http`.
  - Added SSE endpoint discovery (`event: endpoint`) + async JSON-RPC response handling for legacy `sse` transport.
- Expanded MCP server config schema (`internal/mcp/loader.go`):
  - New fields: `transport`, `url`, `messageUrl`, `headers`, `timeoutSec`.
- Added coverage for all supported transports (`internal/mcp/loader_test.go`):
  - stdio helper process test.
  - streamable_http test via `httptest` server.
  - sse test via `httptest` SSE + message endpoint.
- Updated MCP docs/examples:
  - `mcp.json.example` includes stdio + streamable_http + sse samples.
  - `README.md` MCP section now lists supported transports and config format.

### Validation
- `go test ./...`: pass
- `go vet ./...`: pass

### Risks / Follow-up
- SSE implementation focuses on request/response flows and endpoint discovery; server-initiated notifications are ignored for now.
- WebSocket MCP transport is not implemented.

### Next Session First Step
- Add optional websocket MCP transport if needed by target servers.

## 2026-03-05 22:24 (Asia/Ho_Chi_Minh)

### Context Loaded
- Branch: `main`
- Tracker status reviewed: yes

### Changes Made
- Applied MCP transport best-practice fixes after review findings in `internal/mcp/client.go`:
  - Decoupled HTTP request timeout from transport-level client timeout to avoid accidental SSE stream termination.
  - Added per-request timeout handling (`withRequestTimeout`) for HTTP POST requests only.
  - Added parsing support for `text/event-stream` response bodies in `streamable_http` path and extraction of matching JSON-RPC response by `id`.
- Extended tests in `internal/mcp/loader_test.go`:
  - Added `TestRegisterFromFileStreamableHTTPEventStreamResponse` to verify streamable HTTP works when response is SSE-framed.
  - Hardened SSE test with low timeout (`timeoutSec=1`) + delayed call to prove stream is not force-closed by request timeout.
- Updated docs wording in `README.md` module list to reflect multi-transport MCP support.

### Validation
- `go test ./internal/mcp -v`: pass
- `go test ./...`: pass
- `go vet ./...`: pass

### Risks / Follow-up
- `streamable_http` currently supports SSE response parsing for matching JSON-RPC responses, but does not expose server-initiated notifications to upper layers yet.

### Next Session First Step
- Add optional surfaced event channel for MCP server notifications (progress/log events) if product needs them.

## 2026-03-06 12:29 (Asia/Ho_Chi_Minh)

### Context Loaded
- Branch: `main`
- Last commit: `01bd049`
- Tracker status reviewed: yes

### Changes Made
- Added built-in `shell` tool in `internal/tools`:
  - executes `/bin/sh -lc <command>`
  - defaults to workspace root as `cwd`
  - rejects `cwd` paths that escape the workspace
  - supports bounded `timeoutSec`
  - captures bounded `stdout`/`stderr` and returns structured exit status
- Registered `shell` at daemon startup next to `time_now`.
- Replaced stale `browser_open` test expectations with the real built-in `shell` tool:
  - runtime `/tools list` coverage now checks `shell [allowed]`
  - WS `tools.catalog` integration now asserts `shell`
- Added coverage for shell tool behavior:
  - successful execution in workspace
  - `cwd` traversal rejection
  - non-zero exit code surfaced without tool failure
  - timeout handling
- Updated docs:
  - `README.md` built-in tools section
  - `workspace/main/TOOLS.md` registered tools and shell usage notes
  - `docs/PROJECT_TRACKER.md` current state for built-in local tools

### Validation
- `go test ./internal/tools ./internal/agentruntime ./internal/gateway`: pass
- `go test ./...`: pass
- `go vet ./...`: pass

### Risks / Follow-up
- `shell` currently constrains the starting working directory and runtime bounds, but it is not a real sandbox yet; stronger execution policy remains part of P4.
- Example configs still keep `tools.allow` minimal, so `shell` must be explicitly allowlisted before runtime use.

### Next Session First Step
- Start P4 plugin/runtime policy work by defining the command-execution sandbox contract that future local tools can share.

## 2026-03-05 22:24 (Asia/Ho_Chi_Minh)

### Context Loaded
- Branch: `main`
- Tracker status reviewed: yes

### Changes Made
- Applied additional MCP runtime hardening and compatibility updates:
  - Added support for `type` as an alias of `transport` in `mcp.json` server entries.
  - Added alias `streamable-http` and `http` -> `streamable_http` transport resolution.
  - Improved transport inference when `transport/type` is omitted: prefer `sse` if `messageUrl` is provided; otherwise infer `streamable_http` for URL-based servers.
- Improved pending-request failure behavior on transport shutdown:
  - `stdio` and `sse` reader loops now fail pending RPC calls when stream ends/errors.
  - `stdio` wait loop now fails pending RPC calls even when process exits cleanly.
- Added regression coverage:
  - `TestRegisterFromFileTypeAliasHTTP` validates `type: "http"` works end-to-end.
- Updated README MCP notes:
  - Documented `transport`/`type` compatibility and `http` alias behavior.

### Validation
- `go test ./...`: pass
- `go vet ./...`: pass

### Risks / Follow-up
- WebSocket MCP transport is still not implemented; current scope covers stdio + streamable HTTP + legacy SSE.

### Next Session First Step
- Add optional MCP WebSocket transport only if target servers require it in production.
