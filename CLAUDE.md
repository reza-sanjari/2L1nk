# 2L1nk — CLAUDE.md

## Project Overview

**2L1nk** is a self-hosted, end-to-end encrypted communication system for secure real-time messaging and voice calls via a web browser. It is a school project (Berufsschule) built with a zero-knowledge architecture: the server acts strictly as a transport and coordination layer and **cannot decrypt any user content**.

- **Backend:** Go 1.25.2, Echo v4, Gorilla WebSocket, SQLite
- **Frontend:** Vanilla HTML/CSS/JavaScript with `@noble/curves` and `@noble/hashes`
- **Crypto:** Ed25519 identity keys, X25519 key exchange, AES-256-GCM / ChaCha20-Poly1305 message encryption
- **Deployment:** Single static binary + SQLite file, designed for low-power machines (e.g. Raspberry Pi)

---

## Build, Run & Test

### Prerequisites
- Go 1.25.2+
- `make`

### Common Make Targets

```bash
make build          # Build Linux + Windows binaries → bin/linux/2L1nk, bin/windows/2L1nk.exe
make build-static   # Build statically linked binaries (no CGO)
make run            # Run the Linux binary
make test           # Run all tests with race detector
make test-verbose   # Run tests verbosely
make test-api       # API tests only  (tests/api/)
make test-db        # DB tests only   (tests/db/)
make coverage       # Terminal coverage report
make coverage-html  # Generate coverage.html
make fmt            # gofmt all Go files
make lint           # go vet
make tidy           # go mod tidy
make clean          # Remove build artifacts and coverage files
make help           # List all targets
```

### Running the Server

```bash
PORT=8080 DB_PATH=2L1nk.db ./bin/linux/2L1nk
```

On startup the server will:
1. Load config from env vars
2. Open / migrate the SQLite database
3. Generate and print a **gate token** to stdout (required for client auth)
4. Serve HTTP + WebSocket on the configured port
5. Serve the static frontend from `./web/pages/`

### Environment Variables

| Variable | Default    | Purpose                          |
|----------|------------|----------------------------------|
| `PORT`   | `8080`     | HTTP listen port                 |
| `DB_PATH`| `2L1nk.db` | SQLite database file path        |

### Dev Gate Token (tests only)
```
0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
```

---

## Architecture

### Layer Overview

```
cmd/2L1nk/main.go
└── internal/app/app.go          ← Composition root / DI wiring
    ├── internal/api/             ← HTTP layer (Echo handlers, routes, middleware)
    ├── internal/service/         ← Business logic (no DB or HTTP knowledge)
    ├── internal/infrastructure/db/ ← Repository layer (SQL queries)
    ├── internal/db/              ← SQLite setup & migrations
    ├── internal/hub/             ← Runtime WebSocket coordination hub
    ├── internal/session/         ← In-memory authenticated user store
    ├── internal/gate/            ← Gate token generation & validation
    ├── internal/logger/          ← Structured logging (Zap)
    ├── internal/models/          ← Shared enums (UserMode, WSEventType)
    └── internal/config/          ← Env var loading
```

### Hub Pattern (Central Event Loop)

`internal/hub/hub.go` runs a single goroutine managing all runtime WebSocket state. All operations are performed via channels:

| Channel | Purpose |
|---------|---------|
| `RegisterUser` | Add authenticated user to hub |
| `UnregisterUser` | Remove user (disconnect) |
| `JoinRoom` | Put user into a room |
| `RemoveFromRoom` | Remove user from a room |
| `InboundMessages` | Messages received from clients |
| `Broadcast` | Outbound messages to room members |
| `Events` | Domain events emitted for DB persistence |

### Event Consumer

`internal/app/event_consumer.go` reads from `hub.Events` and calls the appropriate service methods to persist state to SQLite (messages, key rotation events, etc.).

### Service Container

All services are grouped in `service.Container` and injected as a single dependency into handlers. Adding a new service: add it to the container struct and wire it in `app.go`.

---

## Key Concepts

### User Modes

| Mode | Description | DB Persistence |
|------|-------------|----------------|
| **Ephemeral** | Key pair generated in-memory per session, destroyed on tab close | Never written to `users` table |
| **Persistent** | Key pair stored in browser (optionally encrypted), survives sessions | Written to `users` table |

### User Identity

- Identity = **Ed25519 key pair** generated entirely on the client
- Canonical user ID = **fingerprint** = `SHA-256(Ed25519_public_key)` (hex string)
- Private key **never** leaves the client
- Usernames are optional display labels only

### Gate Token

- Random 64-char hex string generated on server startup
- Required as a shared secret for initial authentication (`POST /api/auth/gate`)
- **Not** used for message encryption — access control only
- Rotated by restarting the server
- Printed to stdout at startup

### Rooms

- All communication happens in rooms (DMs are just 2-person rooms)
- Each room has a UUID `id`, optional `name`, and a `host_fp` (admin)
- `current_epoch` increments on every membership change and triggers key rotation
- `key_creator_fp` designates which member generates the next epoch key

### End-to-End Encryption

1. Each room has one symmetric key per epoch
2. Room key is encrypted with each member's **X25519 public key** (converted from Ed25519) — one encrypted copy per member (`room_key_slots` table)
3. Messages encrypted with the symmetric key (AES-256-GCM or ChaCha20-Poly1305)
4. Server stores **only ciphertexts** — cannot decrypt anything

### Key Rotation

Triggered on room join or leave:
1. DB epoch increments, new `key_creator_fp` selected
2. Hub broadcasts rotation event to all room members
3. Key creator generates new room key, encrypts it per-member, submits to `POST /api/rooms/:room_id/epoch-keys`
4. Hub broadcasts key slots to members for local decryption

Key creator selection priority:
1. Online persistent users
2. Online ephemeral users
3. Offline persistent users
4. Offline ephemeral users

---

## Database (SQLite)

Migrations run automatically on startup via `internal/db/`.

### Tables

| Table | Purpose |
|-------|---------|
| `users` | Persistent user identities (`fingerprint` PK) |
| `rooms` | Room metadata |
| `room_members` | Many-to-many: users ↔ rooms |
| `messages` | Encrypted message blobs (indexed by `room_id`, `epoch`) |
| `room_key_slots` | Per-member encrypted copies of room keys (for reconnect re-delivery) |
| `voice_sessions` | Voice call session metadata |
| `voice_participants` | Voice participants (mute state, join/leave times) |
| `gate_tokens` | Reserved (currently unused — gate token is in-memory) |

### Important Design Rules
- Ephemeral users are **never** written to the `users` table
- `messages.sender_fp` is intentionally **not** a foreign key (allows ephemeral senders)
- No plaintext keys are ever stored — only ciphertexts

---

## API Endpoints

### Auth
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/auth/gate` | Gate authorization — issue session token |

### Health
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/health` | Server health check |

### Rooms
| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/rooms` | Create room |
| POST | `/api/rooms/:room_id/users/:user_fp` | Add user to room |
| DELETE | `/api/rooms/:room_id/users/:user_fp` | Remove user from room |
| POST | `/api/rooms/:room_id/epoch-keys` | Submit encrypted epoch key slots |
| GET | `/api/rooms/:room_id/messages` | Fetch encrypted messages |

### Users
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/users/me` | Current user info |
| GET | `/api/users` | All connected users |
| GET | `/api/users/me/rooms` | Rooms the current user belongs to |

### WebSocket
| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/ws` | WebSocket upgrade |

### Static Frontend
- `GET /` → `web/pages/index.html`
- `GET /*` → maps URL path to `.html` file in `web/pages/`

### Test Endpoints (dev only)
- `POST /api/test/rooms` — seed test room data

---

## WebSocket Events

First message after WS upgrade must be an auth message:
```json
{ "type": "auth", "payload": { ... } }
```

After auth, the hub registers the user and re-delivers room key slots and pending messages for persistent users.

Event types are defined in `internal/models/` (`WSEventType` enum).

---

## Authentication Flow

1. Client generates Ed25519 key pair + X25519 public key
2. Client computes `fingerprint = SHA-256(ed25519_public_key)`
3. Client `POST /api/auth/gate` with gate token, public keys, username, mode
4. Server validates gate token, issues `session_id` bound to fingerprint
5. Client opens WebSocket, sends auth message with session ID
6. Server registers user in hub

### Request Signing (HTTP)
Requests carry:
- `Chat-Timestamp` header (Unix seconds)
- `Chat-Signature` header (base64 Ed25519 signature)

Canonical input: `METHOD\nPATH\nTIMESTAMP\nSHA-256(BODY)`

> **TODO:** Signature validation is not yet implemented in `ws.go` (line ~81).

---

## Code Conventions

### Adding a New Endpoint
1. Write handler in `internal/api/handlers/`
2. Register route in `internal/api/routes.go`
3. Add business logic to the appropriate service in `internal/service/`
4. Add data access in `internal/infrastructure/db/` (repository pattern)
5. If the handler needs a new service, add it to `service.Container` and wire it in `internal/app/app.go`

### Adding a New WebSocket Event
1. Add event type constant to `internal/models/`
2. Handle in `internal/hub/hub.go` event loop
3. Emit domain event to `hub.Events` if DB persistence is needed
4. Handle in `internal/app/event_consumer.go`

### Adding a New DB Table
1. Add migration SQL in `internal/db/`
2. Add repository in `internal/infrastructure/db/`
3. Add service methods in `internal/service/`

### Logging
Use the injected Zap logger. Always use structured fields:
```go
logger.Info("message", zap.String("key", value), zap.Error(err))
```

### Error Handling
- Return typed errors from services
- Handlers translate to HTTP status codes via Echo's error handling
- Do not log and return — choose one

---

## Project Structure (Quick Reference)

```
cmd/2L1nk/main.go              Entry point
internal/app/app.go            DI wiring / composition root
internal/app/event_consumer.go Hub → DB persistence bridge
internal/api/routes.go         Route registration
internal/api/middleware.go     Auth + logging middleware
internal/api/handlers/         One file per resource group
internal/service/              Business logic (no DB/HTTP)
internal/infrastructure/db/    SQL repositories
internal/db/                   Migrations & DB init
internal/hub/hub.go            WebSocket hub (event loop)
internal/session/              In-memory session store
internal/gate/                 Gate token logic
internal/models/               Shared enums / types
internal/config/               Env var config
web/pages/                     Frontend HTML pages
web/css/                       Stylesheets
web/js/                        Client-side JavaScript
api-spec/openapi.yaml          OpenAPI 3.1.0 spec
api-spec/asyncapi.yaml         AsyncAPI WebSocket spec
docs/                          Architecture & design docs
tests/api/                     HTTP endpoint tests
tests/db/                      Database layer tests
makefile                       All build/test/lint targets
```

---

## Known TODOs

- **Signature validation** in WebSocket handler (`internal/hub/` or `internal/api/handlers/ws.go` ~line 81) — currently skipped
- **Voice call logic** — DB schema exists (`voice_sessions`, `voice_participants`), implementation pending
- **Gate token persistence** — `gate_tokens` table exists but token is currently in-memory only
- **Index optimization** — add indexes for frequent query patterns on `messages` and `room_members`
- **Graceful shutdown** — server does not currently drain connections before exit

---

## External Documentation

- `docs/project_overview.md` — Full architecture & design overview
- `docs/database_schema.md` — Detailed schema with all columns and constraints
- `docs/security.md` — Cryptographic design and threat model
- `docs/strcuture.md` — Directory structure reference
- `api-spec/openapi.yaml` — Full REST API spec
- `api-spec/asyncapi.yaml` — WebSocket event spec
- `tests/postman/` — Postman collection and environment for manual testing
