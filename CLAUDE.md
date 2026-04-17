# 2L1nk — CLAUDE.md

Self-hosted E2E encrypted chat + voice. School project. Zero-knowledge: server never decrypts content.

**Stack:** Go 1.25.2, Echo v4, Gorilla WebSocket, SQLite — Vanilla JS frontend — Ed25519/X25519/AES-256-GCM

## Build & Run

```bash
make build    # → bin/linux/2L1nk
make test     # race detector
make fmt && make lint
PORT=8080 DB_PATH=2L1nk.db ./bin/linux/2L1nk          # TUI (default)
PORT=8080 DB_PATH=2L1nk.db ./bin/linux/2L1nk --server # headless server
```

Dev gate token (tests only): `0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef`

## Architecture

```
cmd/2L1nk/main.go              ← entry point; routes to TUI or --server/--tempserver
internal/app/app.go            ← DI wiring (composition root)
internal/app/event_consumer.go ← hub.Events → DB persistence
internal/api/handlers/         ← HTTP + WS handlers
internal/api/routes.go         ← URL mapping + middleware stack
internal/service/              ← business logic (no DB/HTTP)
internal/infrastructure/db/    ← SQL repositories
internal/hub/hub.go            ← WebSocket event loop (single goroutine, channels)
internal/gate/gate.go          ← token validation, rotation, max-uses, DB sync
internal/cli/                  ← BubbleTea TUI (server mgmt, gate, tunnels, logs)
internal/session/              ← in-memory connected-user store
internal/models/               ← enums (UserMode, WSEventType)
web/                           ← frontend (embedded in binary via go:embed)
```

New endpoint: handler → `routes.go` → service → repo → wire in `app.go`.
New WS event: add to `models/` → handle in `hub_handler.go` → persist in `event_consumer.go`.
New DB table: migration in `internal/db/` → repo → service.

## Key Rules

- **Ephemeral users** are stored in DB (`users` + `room_members`) just like persistent users; only their messages are skipped (not persisted). `messages.sender_fp` is intentionally not a FK
- **No plaintext keys stored** — server only sees ciphertexts
- User identity = `fingerprint = SHA-256(ed25519_public_key)`
- Room key rotates on every join/leave (epoch increments, `key_creator_fp` generates new key)
- Gate is constructed in `main`, passed to `app.New()` — CLI and server share the same DB-backed token
- Services must NOT depend on Hub; use hub.Events + EventConsumer for hub→DB persistence
- Logging: always use structured Zap fields — `zap.String(...)`, `zap.Error(...)`
- Error handling: return typed errors from services, translate to HTTP in handlers — don't log AND return
