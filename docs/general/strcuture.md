# 📁 Project Structure

```txt
2L1nk/
├── cmd/
│   └── 2L1nk/
│       └── main.go
│
├── internal/
│
│   ├── app/                     # Composition root (dependency wiring)
│   │   ├── app.go
│   │   └── event_consumer.go    # Wires hub events → services for DB persistence
│
│   ├── server/                  # HTTP server setup & lifecycle
│   │   └── server.go
│
│   ├── api/                     # HTTP layer
│   │   ├── handlers/            # HTTP → Service translation
│   │   │   ├── handler.go       # Handler struct + constructor
│   │   │   ├── health.go
│   │   │   └── ...
│   │   ├── routes.go            # URL → handler mapping
│   │   └── middleware.go        # Echo middleware
│
│   ├── service/                 # Business logic layer
│   │   ├── container.go         # Service container (all services bundled)
│   │   ├── health_service.go
│   │   ├── gate_service.go
│   │   ├── room_service.go
│   │   ├── message_service.go
│   │   └── ...
│
│   ├── infrastructure/          # External world adapters
│   │   ├── db/                  # Repository implementations
│   │   │   ├── health_repository.go
│   │   │   ├── user_repository.go
│   │   │   ├── room_repository.go
│   │   │   └── message_repository.go
│   │   │
│   │   └── network/             # Networking utilities (IP, STUN, UPnP)
│   │       └── ...
│
│   ├── db/                      # Database setup & migrations (not repositories)
│   │   ├── sqlite.go            # Open DB, configure pragmas, run migrations
│   │   ├── migrations.go        # Schema migrations
│   │   └── setup.go             # Setup + table verification
│
│   ├── hub/                     # Runtime coordination (WebSocket hub)
│   │   ├── hub.go               # Hub struct, Room struct, channel definitions
│   │   ├── hub_handler.go       # Event loop (Run)
│   │   ├── hub_utils.go
│   │   ├── user.go              # hub.User (active WS connection)
│   │   ├── payloads.go          # WS message types and hub request types
│   │   └── events.go
│
│   ├── session/                 # Connected user state (runtime only)
│   │   └── store.go             # In-memory session store
│
│   ├── gate/                    # Access control primitive
│   │   └── gate.go              # Gate token validation and rotation
│
│   ├── logger/                  # Structured logging (Zap wrapper)
│   │   └── logger.go
│
│   ├── utils/                   # Shared utilities
│   │   └── crypto.go            # Fingerprint helpers
│
│   ├── models/                  # Shared enums and types
│   │   └── model.go             # UserMode, WSEventType
│
│   └── config/                  # Configuration loading
│       └── config.go
│
├── web/                         # Frontend (served statically)
│   ├── pages/
│   │   ├── index.html
│   │   ├── login.html
│   │   └── dashboard.html
│   │
│   ├── css/
│   │   └── main.css
│   │
│   └── js/
│       ├── api.js
│       ├── auth.js
│       ├── chat.js
│       └── dashboard.js
│
├── bin/
│   ├── linux/
│   └── windows/
│
├── research/
├── go.mod
├── go.sum
├── makefile
└── README.md
```

---

# 🔹 Backend Architecture Flow

```
main
  ↓
app (dependency injection)
  ↓
server (Echo setup)
  ↓
routes (URL mapping)
  ↓
handlers (HTTP layer)
  ↓
services (business logic)
  ↓
infrastructure (DB, network, external systems)
```

Each layer depends only downward.

---

# 🧩 Major Component Responsibilities

---

## `cmd/2L1nk/main.go`

* Entry point only
* Loads config
* Creates app
* Starts app
* No business logic

---

## `internal/app/`

* Composition root
* Instantiates:

    * Infrastructure
    * Services
    * Handlers
    * Server
* Performs dependency injection
* Nothing else

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

* Repository implementations (UserRepository, RoomRepository, MessageRepository, etc.)
* Each repository implements the interface defined in its corresponding service file

### `db/` (setup — `internal/db/`)

* SQLite connection setup
* WAL mode and pragma configuration
* Schema migrations

### `network/`

* Public IP detection
* UPnP handling
* STUN
* Connectivity logic

Rule:
If it talks to the outside world → infrastructure.

---

## `internal/hub/`

* WebSocket connection manager
* In-memory routing
* Real-time coordination
* No HTTP logic

---

## `internal/models/`

* Pure structs
* Shared across layers
* No DB logic
* No service logic

---

## `internal/config/`

* Load env variables
* Define Config struct
* No runtime logic

---

# 🌐 Frontend (`web/`)

Pure static frontend.

Echo serves it as static files.

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