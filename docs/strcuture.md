# 📁 Project Structure

```txt
2L1nk/
├── cmd/
│   └── 2L1nk/
│       └── main.go              # Entry point; routes to TUI or --server / --tempserver
│
├── internal/
│
│   ├── app/                     # Composition root (dependency wiring)
│   │   ├── app.go
│   │   └── event_consumer.go    # Wires hub.Events → services for DB persistence
│
│   ├── server/                  # HTTP server setup & lifecycle
│   │   └── server.go            # Echo instance, embedded static file serving, routes
│
│   ├── api/                     # HTTP layer
│   │   ├── handlers/            # HTTP → Service translation
│   │   │   ├── handler.go       # Handler struct + constructor
│   │   │   ├── health.go
│   │   │   ├── gate.go
│   │   │   ├── ws.go            # WebSocket upgrade handler
│   │   │   ├── NewRoom.go
│   │   │   ├── addUserToRoom.go
│   │   │   ├── removeUserFromRoom.go
│   │   │   ├── getUserRooms.go
│   │   │   ├── getRoomMessages.go
│   │   │   ├── getKeySlots.go
│   │   │   ├── epochKeys.go
│   │   │   ├── userInfo.go
│   │   │   ├── allUsers.go
│   │   │   ├── room_helpers.go
│   │   │   └── testRooms.go
│   │   ├── routes.go            # URL → handler mapping + middleware stack
│   │   └── middleware.go        # AuthMiddleware (session-based)
│
│   ├── service/                 # Business logic layer
│   │   ├── container.go         # Service container (all services bundled)
│   │   ├── health_service.go
│   │   ├── gate_service.go
│   │   ├── room_service.go
│   │   ├── message_service.go
│   │   └── errors.go
│
│   ├── infrastructure/          # External world adapters
│   │   └── db/                  # Repository implementations
│   │       ├── gate_repository.go
│   │       ├── health_repository.go
│   │       ├── user_repository.go
│   │       ├── room_repository.go
│   │       └── message_repository.go
│
│   ├── db/                      # Database setup & migrations (not repositories)
│   │   ├── sqlite.go            # Open DB, configure pragmas, run migrations
│   │   ├── migrations.go        # Schema migrations
│   │   └── setup.go             # Setup + table verification
│
│   ├── hub/                     # Runtime coordination (WebSocket hub)
│   │   ├── hub.go               # Hub struct, Room struct, channel definitions
│   │   ├── hub_handler.go       # Event loop (Run) — single goroutine, select on channels
│   │   ├── hub_utils.go         # Internal helpers
│   │   ├── user.go              # hub.User (active WS connection)
│   │   ├── payloads.go          # WS message types and hub request/event payloads
│   │   └── events.go            # HubEvent types
│
│   ├── session/                 # Connected user state (runtime only)
│   │   └── store.go             # In-memory session store
│
│   ├── gate/                    # Access control primitive
│   │   └── gate.go              # Gate token validation, rotation, max-uses, DB sync
│
│   ├── cli/                     # Interactive terminal UI (BubbleTea)
│   │   ├── cli.go               # TUI entry point (RunTUI)
│   │   ├── model.go             # Root BubbleTea model
│   │   ├── menu.go              # Main menu
│   │   ├── gate_menu.go         # Gate key management screen
│   │   ├── gate_history.go      # Gate token history view
│   │   ├── tunnel.go            # Tunnel config types, presets, file I/O
│   │   ├── tunnel_menu.go       # Tunnel list screen
│   │   ├── tunnel_detail.go     # Tunnel detail / start-stop screen
│   │   ├── tunnel_add.go        # Add new tunnel screen
│   │   ├── tunnel_log_view.go   # Live tunnel log view
│   │   ├── options.go           # Options types and file I/O
│   │   ├── options_menu.go      # Options screen
│   │   ├── logs_view.go         # Server log viewer
│   │   ├── nuke.go              # Nuke (wipe all data) action
│   │   ├── reset.go             # Reset database action
│   │   ├── proc_unix.go         # Process management (Unix)
│   │   ├── proc_windows.go      # Process management (Windows)
│   │   └── theme.go             # Lipgloss styles / color theme
│
│   ├── logger/                  # Structured logging (Zap wrapper)
│   │   └── logger.go
│
│   ├── utils/                   # Shared utilities
│   │   ├── crypto.go            # Fingerprint helpers
│   │   └── secure_delete.go     # Overwrite-then-delete for sensitive files
│
│   ├── models/                  # Shared enums and types
│   │   └── model.go             # UserMode, WSEventType
│
│   └── config/                  # Configuration loading
│       └── config.go            # PORT, DB_PATH from environment
│
├── web/                         # Frontend (embedded in binary at build time)
│   ├── pages/
│   │   ├── Login.html
│   │   ├── index.html
│   │   ├── Chat.html
│   │   ├── Mainsite.html
│   │   ├── 404.html
│   │   └── ...
│   │
│   ├── css/
│   │   ├── main.css
│   │   ├── login.css
│   │   └── mainsite.css
│   │
│   └── js/
│       ├── app.js
│       └── mainsite.js
│
├── bin/
│   ├── linux/
│   └── windows/
│
├── docs/
├── go.mod
├── go.sum
├── makefile
└── README.md
```

---

# 🔹 Entry Point Modes

`main.go` routes to one of three modes:

```
./2L1nk                   → TUI (BubbleTea CLI)
./2L1nk --server          → run server directly (writes PID + log file)
./2L1nk --tempserver      → ephemeral server (no log file; deletes DB on exit)
```

The TUI spawns the server as a subprocess (`--server` with env flags) and manages its lifecycle via a PID file.

# 🔹 Backend Architecture Flow

```
main (--server mode)
  ↓
app (composition root / dependency wiring)
  ↓
server (Echo setup, embedded static files)
  ↓
routes (URL mapping + middleware stack)
  ↓
handlers (HTTP/WS layer)
  ├── services (business logic)
  │     └── infrastructure (DB repositories)
  └── hub (runtime WS coordination)
        └── event_consumer (hub.Events → services → DB)
```

Each layer depends only downward. The handler is the only layer allowed to touch both services and hub.

---

# 🧩 Major Component Responsibilities

---

## `cmd/2L1nk/main.go`

* Entry point only
* Routes to TUI mode (default) or server mode (`--server` / `--tempserver`)
* In server mode: loads config, creates gate, creates app, writes PID file, starts app
* In `--tempserver` mode: suppresses stdout logging, securely deletes DB on exit
* No business logic

---

## `internal/app/`

* Composition root — the only place dependencies are constructed and wired
* Instantiates in order: logger → DB → session store → repos → services → service container → hub → event consumer → handler → server
* `event_consumer.go` runs a goroutine consuming `hub.Events` and persisting to DB via services

---

## `internal/server/`

* Creates Echo instance
* Applies middleware
* Registers routes
* Starts HTTP server
* No business logic
* No service construction

---

## `internal/api/`

### `handlers/`

* Parse request
* Call service
* Return JSON
* No SQL
* No infrastructure logic

### `routes.go`

* Maps URL → handler methods
* No construction
* No logic

### `middleware.go`

* Logging
* Rate limiting
* Authentication middleware

---

## `internal/service/`

* Core business logic
* Orchestrates repositories and infrastructure
* No HTTP awareness
* No Echo dependency

---

## `internal/infrastructure/`

Everything external.

### `db/` (repositories — `internal/infrastructure/db/`)

* Repository implementations: GateRepository, HealthRepository, UserRepository, RoomRepository, MessageRepository
* Each repository implements the interface defined in its corresponding service or gate file

### `db/` (setup — `internal/db/`)

* SQLite connection setup
* WAL mode and pragma configuration
* Schema migrations

Rule:
If it talks to the outside world → infrastructure.

---

## `internal/hub/`

* WebSocket connection manager
* In-memory routing
* Real-time coordination
* No HTTP logic

---

## `internal/cli/`

* Interactive terminal UI built with BubbleTea
* Manages server subprocess lifecycle via PID file (start / stop)
* Screens: main menu, gate key management, gate history, tunnel management, options, log viewer, reset, nuke
* Tunnel subsystem: configure and run outbound tunnels (Cloudflare, SSH-based, etc.) with live log tailing
* Shares the same DB as the server so gate key changes take effect immediately without restart

---

## `internal/gate/`

* Access control primitive — DB-backed with in-memory cache
* Validates gate tokens, tracks use count, auto-rotates on max-uses
* Syncs active token from DB on each validate call (CLI changes apply without server restart)

---

## `internal/models/`

* Pure enums and types shared across layers
* `UserMode` (persistent / ephemeral), `WSEventType`
* No DB logic, no service logic

---

## `internal/config/`

* Load env variables (`PORT`, `DB_PATH`)
* Define Config struct
* No runtime logic

---

## `internal/utils/`

* `crypto.go` — fingerprint helpers (SHA-256 of public key)
* `secure_delete.go` — overwrite file with zeros then delete (used by `--tempserver` cleanup)

---

# 🌐 Frontend (`web/`)

Pure static frontend.

The entire `web/` directory is embedded into the binary at build time via Go's `embed` package. Echo serves it from memory — no separate static file server needed.

### `pages/`

HTML entry points.

### `js/`

Client-side logic:

* API calls
* WebSocket handling
* Encryption logic
* DOM updates

### `css/`

Styling only.

Backend returns JSON only.
Frontend owns UI.

---

# 🔐 Naming Rules Going Forward

* `repository` = persistence only (DB)
* Everything else external → `infrastructure`
* Business logic → `service`
* HTTP translation → `handler`
* Wiring → `app`
* Server lifecycle → `server`