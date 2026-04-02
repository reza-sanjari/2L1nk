# Dependency Architecture

## Overview

The application uses a **Composition Root + Service Container** pattern.

All dependencies are constructed exactly once inside:

```txt
internal/app/app.go
```

No other layer is allowed to construct cross-layer dependencies.

The system has **five major pillars**:

1. **Logger** (cross-cutting concern)
2. **Service Layer** (business logic)
3. **Hub Layer** (runtime WebSocket coordination)
4. **Session Layer** (connected user state)
5. **CLI/TUI Layer** (operator management — runs outside the server process)

Handlers act as the bridge between the Service layer and the Hub layer.

The Event Consumer bridges Hub and Services for DB persistence without violating the dependency rules.

---

## Global Dependency Flow

```txt
main
  ├── [default]  → cli (TUI process)
  │                   └── gate (shared DB, in-memory cache)
  │
  └── [--server] → app (composition root)
                      ├── logger (cross-cutting)
                      ├── session (runtime identity store)
                      ├── infrastructure (repos/adapters)
                      ├── gate (access control primitive, passed from main)
                      ├── services (business logic)
                      ├── service container (bundles services)
                      ├── hub (runtime coordination)
                      ├── event consumer (hub.Events → services → DB)
                      ├── handlers (transport orchestration)
                      └── server (Echo lifecycle + middleware + embedded assets)
```

Important:

- Services do NOT depend on Hub
- Hub does NOT depend on Services
- Handler is the only layer allowed to talk to both
- Event Consumer reads hub.Events and calls services — it is the only sanctioned path from hub to DB
- Gate is constructed in `main` and passed into `app.New()` so the CLI and server share the same token
- Logger may be used across layers (as a cross-cutting dependency)

---

## Runtime vs Business Separation

The architecture intentionally separates:

### ❄ Business World

- Services
- Models
- Repositories
- Gate logic
- Persistence decisions

### 🔥 Runtime World

- Hub
- Session store
- Active WebSocket connections
- Room membership

The Handler coordinates between these two worlds.

---

# Layer Breakdown

---

## Logger (`internal/logger/`)

A cross-cutting concern constructed in the composition root and injected where needed.

Responsibilities:

- Structured logging (Zap)
- Central config (level, JSON vs console, output file)
- Process-wide lifecycle (`Sync()` on shutdown)

Usage rules:

- Logger is safe to inject into services and infrastructure (and optionally handlers/server) for observability.
- Avoid putting business decisions into logging; it should remain side-effect only.

---

## Infrastructure (`internal/infrastructure/`)

Adapters for the external world.

Examples:

- SQLite repositories
- Network utilities
- STUN / UPnP
- External integrations

Repositories live here.

Services define the interfaces they depend on.

Infrastructure implements them.

---

## Session Layer (`internal/session/`)

Manages **connected users only**.

Responsibilities:

- Track active sessions
- Map `sessionID → connected user`
- Enforce username uniqueness (runtime)
- Remove sessions on disconnect

It does NOT:

- Manage rooms
- Contain business logic
- Persist users

Session state is purely runtime and disappears on shutdown.

---

## Hub Layer (`internal/hub/`)

Manages real-time coordination.

Responsibilities:

- Track WebSocket connections
- Manage in-memory rooms
- Handle join/leave
- Broadcast messages
- React to disconnect events
- Emit `HubEvent`s for DB-side persistence (consumed by Event Consumer)

Hub receives:

- `session.Store`
- `*logger.Logger`

Hub does NOT:

- Access database
- Enforce business rules
- Validate permissions

---

## Event Consumer (`internal/app/event_consumer.go`)

A goroutine that reads `hub.Events` (a buffered channel) and persists state changes to the database via services.

Responsibilities:

- Persist new rooms (`HubEventRoomCreated`)
- Persist messages (`HubEventMessageCreated`)
- Handle offline-room delivery: fetch room from DB and re-inject into hub (`HubEventRoomOffline`)
- Persist epoch/key-creator updates after hub-internal key rotation (`HubEventKeyRotationTriggered`)
- Persist epoch key slots (`HubEventEpochKeysSubmitted`)

This is the only path where hub activity reaches the database. It satisfies the "Hub does not depend on Services" rule by inverting the call direction via a channel.

---

## Gate Component (`internal/gate/`)

Gate is constructed in the composition root and injected into `GateService`.

It is pure access-control/business logic.

It does not know about HTTP or WebSockets.

---

## Service Layer (`internal/service/`)

Contains business logic.

Examples:

- HealthService
- GateService
- RoomService
- MessageService

Services may depend on:

- Repositories
- Gate logic
- (If required) session abstractions
- Logger (cross-cutting)

Services must NOT depend on:

- Hub
- Echo
- WebSockets
- HTTP

Services are transport-agnostic.

---

## Service Container (`internal/service/container.go`)

Bundles all services.

Constructed once in `app.go`.

Example shape:

```go
type Container struct {
    Health  *HealthService
    Gate    *GateService
    Room    *RoomService
    Message *MessageService
}

func NewContainer(health *HealthService, gate *GateService, room *RoomService, message *MessageService) *Container {
    return &Container{
        Health:  health,
        Gate:    gate,
        Room:    room,
        Message: message,
    }
}
```

Handlers receive this container.

---

## Handler (`internal/api/handlers/`)

Handler is the orchestration layer.

It receives:

- `*service.Container`
- `*hub.Hub`
- `*session.Store`
- `*logger.Logger`

Responsibilities:

- Parse HTTP / WebSocket requests
- Call services for business logic
- Call hub for runtime coordination
- Return JSON responses

Handler is the only layer allowed to interact with both Service and Hub.

---

## Server (`internal/server/`)

Creates Echo instance.

Receives:

- `*handlers.Handler`
- `*session.Store` (for middleware)

Responsibilities:

- Register routes
- Apply middleware
- Start HTTP server
- Handle lifecycle

Server does NOT:

- Contain business logic
- Construct services
- Know about infrastructure

---

## App (`internal/app/app.go`)

The composition root.

This is the only place where dependencies are constructed and wired.

`Gate` is constructed in `main` (before calling `app.New`) so both the TUI and the server share state through the same DB-backed gate token.

Current wiring order (as of the latest `app.go`):

1. Logger
2. Database
3. Gate repo attached to the pre-constructed Gate (syncs with DB)
4. Session Store
5. Infrastructure (healthRepo, roomRepo, msgRepo, userRepo)
6. Services (inject infrastructure + logger; gate service also gets session store + userRepo)
7. Service Container
8. Hub (receives Session Store + Logger)
9. Event Consumer (goroutine: hub.Events → services → DB)
10. Handler (receives Container + Hub + Session Store + Logger)
11. Server (receives Config + Handler + Session Store; serves embedded assets)

Current `app.go` wiring sketch:

```go
func New(cfg *config.Config, g *gate.Gate, logFile string, suppressStdout bool) *App {
    // 1. Logger
    logg, _ := logger.New(logger.Config{..., OutputFile: logFile, SuppressStdout: suppressStdout})

    // 2. Database
    database, _ := db.Setup(cfg.DBPath, logg)

    // 3. Gate repo
    gateRepo := infradb.NewGateRepository(database)
    _ = g.SetRepo(gateRepo)

    // 4. Session
    sessionStore := session.NewStore()

    // 5. Infrastructure
    healthRepo := infradb.NewHealthRepository(database)
    roomRepo   := infradb.NewRoomRepository(database)
    msgRepo    := infradb.NewMessageRepository(database)
    userRepo   := infradb.NewUserRepository(database)

    // 6. Services
    healthSvc := service.NewHealthService(healthRepo, logg)
    gateSvc   := service.NewGateService(g, sessionStore, userRepo, logg)
    roomSvc   := service.NewRoomService(roomRepo, logg)
    msgSvc    := service.NewMessageService(msgRepo, roomRepo, logg)

    // 7. Container
    services := service.NewContainer(healthSvc, gateSvc, roomSvc, msgSvc)

    // 8. Hub
    mainHub := hub.New(sessionStore, logg)
    go mainHub.Run()

    // 9. Event Consumer
    startEventConsumer(mainHub, roomSvc, msgSvc, logg)

    // 10. Handler
    handler := handlers.NewHandler(services, mainHub, sessionStore, logg)

    // 11. Server
    srv := server.New(cfg, handler, sessionStore)

    return &App{server: srv, logger: logg}
}
```

Shutdown behavior:

- `App.Start()` and `App.Stop()` both defer `logger.Sync()`
- Signal handling (SIGINT/SIGTERM) is done in `main`, not `app`

---

# Dependency Rules (Strict)

### Allowed

```txt
Handler → Service
Handler → Hub
Handler → Session
Service → Infrastructure
Service → Gate
Hub → Session
Hub →(events chan)→ EventConsumer → Service
Server → Handler
Server → Session (middleware only)
CLI → Gate (shared DB)
(Logger) → can be injected into any layer as a cross-cutting concern
```

### Forbidden

```txt
Service → Hub
Service → Echo
Hub → Service          (use Events channel + EventConsumer instead)
Infrastructure → Service
Session → Service
```

---

# Summary Table

| Layer           | Receives                                        | Provides                          | Injected? |
|----------------|-------------------------------------------------|-----------------------------------|-----------|
| Logger         | Config                                          | Structured logging                | Yes       |
| Infrastructure | DB connection                                   | Repo implementations              | Yes       |
| Session        | —                                               | Connected user registry           | Yes       |
| Gate           | Repo (optional), Logger                         | Token validation & rotation       | Yes       |
| Hub            | Session store, Logger                           | Runtime WS coordination + Events  | Yes       |
| EventConsumer  | Hub (Events chan), RoomSvc, MsgSvc, Logger      | Hub→DB persistence bridge         | No        |
| Service        | Repos, Gate, (optional) session, Logger         | Business logic                    | Yes       |
| Container      | All services                                    | Bundled service access            | Yes       |
| Handler        | Container + Hub + Session + Logger              | Transport orchestration           | Yes       |
| Server         | Config + Handler + Session (middleware)         | HTTP lifecycle + static assets    | No        |
| App            | Config + Gate + log path                        | Dependency wiring                 | —         |
| CLI            | Config + Gate (shared DB)                       | Operator management TUI           | —         |

---

# Architectural Principle

The system maintains a strict separation between:

- **Business logic**
- **Runtime coordination**
- **Transport**
- **Infrastructure**
- **Cross-cutting concerns** (logging)

The Handler is the controlled bridge.

The Hub is runtime-only.

The Service layer is transport-agnostic.

The Session layer is ephemeral.

All wiring happens in one place.
