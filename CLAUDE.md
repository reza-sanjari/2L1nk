# 2L1nk — CLAUDE.md

Self-hosted E2E encrypted chat + voice. School project. Zero-knowledge: server never decrypts content.

**Stack:** Go 1.25.2, Echo v4, Gorilla WebSocket, SQLite — Vanilla JS frontend — Ed25519/X25519/AES-256-GCM

## Build & Run

```bash
make build    # → bin/linux/2L1nk
make test     # race detector
make fmt && make lint
PORT=8080 DB_PATH=2L1nk.db ./bin/linux/2L1nk
```

Dev gate token (tests only): `0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef`

## Architecture

```
cmd/2L1nk/main.go
internal/app/app.go            ← DI wiring
internal/api/handlers/         ← HTTP handlers
internal/service/              ← business logic (no DB/HTTP)
internal/infrastructure/db/    ← SQL repositories
internal/hub/hub.go            ← WebSocket event loop (single goroutine, channels)
internal/app/event_consumer.go ← hub.Events → DB persistence
internal/models/               ← enums (UserMode, WSEventType)
web/pages/ web/css/ web/js/    ← frontend
```

New endpoint: handler → `routes.go` → service → repo → wire in `app.go`.
New WS event: add to `models/` → handle in `hub.go` → persist in `event_consumer.go`.
New DB table: migration in `internal/db/` → repo → service.

## Key Rules

- **Ephemeral users** are never written to `users` table; `messages.sender_fp` is intentionally not a FK
- **No plaintext keys stored** — server only sees ciphertexts
- User identity = `fingerprint = SHA-256(ed25519_public_key)`
- Room key rotates on every join/leave (epoch increments, `key_creator_fp` generates new key)
- Logging: always use structured Zap fields — `zap.String(...)`, `zap.Error(...)`
- Error handling: return typed errors from services, translate to HTTP in handlers — don't log AND return

## Known TODOs

- Signature validation not implemented (`ws.go` ~line 81)
- Voice call logic pending (DB schema exists)
- Graceful shutdown not implemented
