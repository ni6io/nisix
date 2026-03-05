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
