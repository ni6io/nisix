# Session Start Guide

Use this checklist at the beginning of every new coding session.

## 1) Read Order (5-10 minutes)

1. `README.md`
2. `docs/PROJECT_TRACKER.md`
3. `docs/WORK_LOG.md` (latest entry only)
4. `configs/nisix.local.json` (or create from example)
5. `workspace/main/*.md` (agent profile files)

## 2) Environment Sanity

Run from project root:

```bash
cd <repo-root>
go test ./...
```

If using Ollama provider:

```bash
ollama list
curl -sS http://127.0.0.1:11434/api/tags | head -c 200
```

## 3) Start Server

```bash
go run ./cmd/nisixd -config configs/nisix.local.json -listen :18789
```

Expected:

- `/health` returns `{ "ok": true }`
- Telegram polling starts if enabled in config

## 4) Smoke Tests

HTTP:

```bash
curl -s http://127.0.0.1:18789/health
```

Telegram DM:

1. `/onboard status`
2. `/profile list`
3. `my name is <name>`
4. `/profile apply <proposal_id>`
5. `/profile show USER.md`

## 5) Before Ending Session

1. Update `docs/WORK_LOG.md` with what changed and what is next.
2. Update `docs/PROJECT_TRACKER.md` statuses if roadmap moved.
3. Run `go test ./...` again.
4. Commit with a focused message.
