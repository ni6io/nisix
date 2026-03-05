# AGENTS.md

Hướng dẫn nhanh cho agent/dev khi bắt đầu session mới tại `nisix`.

## Mục tiêu dự án

- Xây trợ lý theo hướng OpenClaw bằng Go.
- Kênh chính: Telegram + WS control plane.
- Ưu tiên: ổn định runtime, dễ vận hành, dễ mở rộng skills/tools.

## Bắt đầu session (bắt buộc)

1. Đọc `docs/SESSION_START.md`.
2. Đọc `docs/PROJECT_TRACKER.md`.
3. Đọc entry mới nhất trong `docs/WORK_LOG.md`.
4. Chạy `go test ./...` trước khi sửa code.

## Quy ước kỹ thuật hiện tại

- Bootstrap/runtime:
  - `bootstrap.reloadMode = per_message`
  - context order: `IDENTITY -> SOUL -> AGENTS -> TOOLS -> USER`
  - onboarding state: `<workspace>/.nisix/workspace_state.json`
- Profile:
  - `profile.updateMode = hybrid`
  - lệnh: `/profile list|show|diff|set|append|apply`
  - guardrails: allowlist file + max bytes + atomic write
- Memory:
  - `memory.autoLoadScope = dm_only`
- Models:
  - hỗ trợ `echo`, `openai/codex`, `ollama`
  - local mặc định khuyến nghị: `ollama`

## Run/Test

```bash
cd <repo-root>
cp configs/nisix.local.example.json configs/nisix.local.json
go run ./cmd/nisixd -config configs/nisix.local.json -listen :18789
```

Kiểm tra:

```bash
go test ./...
go vet ./...
curl -s http://127.0.0.1:18789/health
```

## Config & secrets

- Không commit secrets thật vào repo.
- `configs/nisix.local.json` là file local (đã ignore).
- `configs/nisix.example.json` luôn giữ placeholder.

## Khi kết thúc session

1. Cập nhật `docs/WORK_LOG.md`.
2. Nếu roadmap đổi, cập nhật `docs/PROJECT_TRACKER.md`.
3. Chạy lại `go test ./...`.
4. Commit rõ ràng, scope nhỏ, message mô tả đúng thay đổi.
