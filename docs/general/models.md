# Models Reference

This document lists the key types used across the codebase, organized by package.

---

# internal/models/model.go

## UserMode

```go
type UserMode int

const (
    UserModeEphemeral  UserMode = iota // 0 — session only, user record never saved to DB
    UserModePersistent                 // 1 — user record saved to DB, persists across reconnects
)
```

## WSEventType

```go
type WSEventType string

const (
    Auth          WSEventType = "auth"
    Message       WSEventType = "message"
    JoinRoom      WSEventType = "join_room"
    LeaveRoom     WSEventType = "leave_room"
    Signal        WSEventType = "signal"
    Error         WSEventType = "error"
    KeyRotation   WSEventType = "room_key_rotation"
    KeySlot       WSEventType = "room_key_slot"
    EpochMismatch WSEventType = "epoch_mismatch"
)
```

---

# internal/session/store.go

## session.User

Represents a connected user in the session store. Lives only while the WebSocket connection is active.

```go
type User struct {
    SessionID            string
    PublicKey            ed25519.PublicKey
    X25519PublicKey      []byte // raw 32-byte X25519 public key
    PublicKeyFingerprint string
    Username             string
    Mode                 models.UserMode
}
```

---

# internal/hub/user.go

## hub.User

Represents a user with an active WebSocket connection inside the hub.

```go
type User struct {
    Fingerprint      string
    Username         string
    X25519PublicKey  string // base64-encoded X25519 public key
    OutGoingMessages chan []byte
    Websocket        *websocket.Conn
    PeerMux          sync.Mutex
    Mode             models.UserMode
}
```

---

# internal/hub/hub.go

## hub.Room

In-memory room state managed by the hub. Tracks both active connections and all known members.

```go
type Room struct {
    Name             string
    RoomID           string
    Host             *User            // live WS connection; nil when host is offline
    HostFP           string           // always set
    HostName         string
    KeyCreatorFP     string           // fingerprint of current key creator
    Users            map[string]*User // only active WS connections
    Epoch            int64
    MemberPublicKeys map[string]string          // fingerprint → base64 X25519 public key (all known members)
    MemberModes      map[string]models.UserMode // fingerprint → mode (all known members)
    PendingRotation  *PendingRotation           // non-nil while waiting for key creator to submit keys
}
```

---

# internal/hub/payloads.go

## WSMessageEnvelope

WebSocket message wrapper used for all inbound and outbound WS messages.

```go
type WSMessageEnvelope struct {
    Sender  *User              `json:"-"` // server-only, not serialized
    Type    models.WSEventType `json:"type"`
    Payload json.RawMessage    `json:"payload"`
}
```

---

# internal/infrastructure/db/

## UserRecord

Persisted user data. Only written for `UserModePersistent` users.

```go
type UserRecord struct {
    Fingerprint     string // primary key
    PublicKey       string // base64-encoded Ed25519 public key
    X25519PublicKey string // base64-encoded X25519 public key
    Username        string
    CreatedAt       int64  // unix timestamp
}
```

## RoomRecord

Persisted room data.

```go
type RoomRecord struct {
    ID           string
    Name         string
    CurrentEpoch int64
    HostFP       string // empty string when NULL in DB
    KeyCreatorFP string // empty string when NULL in DB
    CreatedAt    int64  // unix timestamp
}
```

## MessageRecord

Persisted encrypted message. Server stores ciphertext only — no decryption is possible server-side.

```go
type MessageRecord struct {
    ID         string
    RoomID     string
    SenderFP   string
    Epoch      int64
    Type       int    // 0=text, 1=system, 2=signal
    Ciphertext string // base64-encoded encrypted payload
    CreatedAt  int64  // unix timestamp
}
```
