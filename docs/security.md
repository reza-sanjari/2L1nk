# Security Overview: Cryptographic Identity & Key Architecture

## 1. Key Pair Model

Each user generates **one key pair** based on **Curve25519**. This single key pair serves all cryptographic needs in the system.

| Algorithm    | Derived From           | Purpose                                  |
|--------------|-----------------------|------------------------------------------|
| **Ed25519**  | Generated directly     | Signatures, authentication, identity     |
| **X25519**   | Converted from Ed25519 | Encrypting room keys to members          |

An Ed25519 key can be mathematically converted to an X25519 key. This is a well-established, safe conversion used by Signal, libsodium, WireGuard, and age.

---

## 2. User Identity

- The **Ed25519 public key** is the user's canonical identity.
- The **public key fingerprint** (`SHA-256(Ed25519_public_key)`) is used for routing and as the user identifier visible to the server.
- The **private key never leaves the client**.

### 2.1 Fingerprints

A fingerprint is the SHA-256 hash of the user's Ed25519 public key.

```
fingerprint = SHA-256(Ed25519_public_key)
```

Fingerprints serve as the primary user identifier within the system:

- **Routing**: The server maps fingerprints to active WebSocket connections and uses them to deliver messages to the correct recipient.
- **Session binding**: During gate authentication, the server associates the submitted fingerprint with the issued session ID.
- **Identity verification**: Users can compare fingerprints out-of-band (QR code, voice call, in person) to confirm they are communicating with the intended party.
- **Key change detection**: If a user's public key changes (new device, ephemeral mode reload), the fingerprint changes. Other participants can detect this as a potential identity change.

---

## 3. Cryptographic Operations by Use Case

### 3.1 Gate Authentication

The client proves ownership of its key pair during the gate authorization step.

```
Client:  signature = Ed25519_Sign(private_key, gateToken || timestamp)
Server:  Ed25519_Verify(public_key, gateToken || timestamp, signature)
```

The server verifies the signature using the public key submitted in the request. If verification succeeds, a session ID is issued. The private key is never transmitted.

### 3.2 Room Key Distribution (All Rooms)

All rooms — regardless of member count — use the same key distribution model. There is no distinction between DMs and group chats at the cryptographic layer.

The room creator generates a random symmetric key and distributes it to each member individually.

```
room_key = random AES-256 key

For each member:
    encrypted_room_key = encrypt(member_X25519_public_key, room_key)
    send encrypted_room_key to member

Member:
    room_key = decrypt(own_X25519_private_key, encrypted_room_key)
```

- All members use the same `room_key` to encrypt and decrypt messages using a symmetric AEAD cipher (AES-256-GCM or ChaCha20-Poly1305).
- On membership change, the room key is rotated and redistributed (see 3.3).
- The server never sees the room key or plaintext.
- ECDH is not used. X25519 is used only for encrypting the room key to each member's public key.

### 3.3 Key Rotation and Epochs

Every room maintains an **epoch counter** that increments on each key rotation. Key rotation occurs when:

- A member **joins** the room (new member must not read old messages).
- A member **leaves** the room (departed member must not read future messages).

The **server** is responsible for coordination: it updates the member list, increments the epoch, selects the key creator, and broadcasts a rotation event to all members.

```
Member leaves or joins room
    → Server removes/adds member
    → Server increments epoch
    → Server selects key_creator using 4-tier priority (see section 4.2)
    → Server broadcasts RoomKeyRotationEvent to all online members
```

```json
{
  "type": "room_key_rotation",
  "payload": {
    "room_id":        "string",
    "epoch":          int64,
    "key_creator_fp": "string",
    "members": [
      { "fingerprint": "string", "x25519_public_key": "string (base64)" }
    ]
  }
}
```

The `members` array includes the X25519 public key of every known room member (online and offline persistent). The key creator uses these keys directly to encrypt the new room key without any additional lookup.

The designated key creator then:

```
room_key_N = random AES-256 key
for each member:
    encrypted_room_key = encrypt(member_X25519_public_key, room_key_N)
    send (encrypted_room_key, epoch=N)
```

Other members wait for the encrypted key from the designated key creator, then decrypt it with their own X25519 private key.

Messages include the epoch number so recipients know which key to use for decryption.

### 3.4 Message Persistence and Historical Decryption

The server stores encrypted messages as opaque blobs. Each stored message includes the epoch under which it was encrypted.

```
StoredMessage {
    id:          string
    room_id:     string
    sender_fp:   string
    epoch:       uint64    // which room key was used
    ciphertext:  bytes
    timestamp:   int64
}
```

The server **cannot decrypt** any stored message. It is a dumb storage layer.

The client maintains a **local key store** mapping `(room_id, epoch) → room_key`. This key store is persisted in the browser (IndexedDB), optionally encrypted with the user's passphrase.

```
On reconnect / loading history:
    1. Client authenticates and connects
    2. Server sends stored encrypted messages for the user's rooms
    3. Client looks up the correct room key by (room_id, epoch) from local key store
    4. Client decrypts each message using the corresponding epoch key
```

Key retention rules:

- Old epoch keys are **never deleted** from the client key store (needed for historical decryption).
- Old epoch keys must **not** be used for encrypting new messages — only the current epoch key is used for sending.
- If the client's local key store is lost (cleared storage, new device), messages from previous epochs **cannot be decrypted**. This is a deliberate trade-off: simplicity and forward secrecy over cross-device history sync.

### 3.5 Voice Calls

- WebRTC handles encryption natively (DTLS-SRTP).
- The server is used only for signaling (SDP/ICE exchange).
- Signaling messages may optionally be encrypted using room keys.

---

## 4. Key Exchange Protocol: Complete Flow

This section documents every scenario in which keys are generated, rotated, or delivered, traced to the actual server implementation.

---

### 4.1 Room Creation — Epoch 0

```
Client:  POST /api/rooms  { groupName }
Server:  hub.RegisterRoom ← CreateRoomRequest
```

1. Hub creates the room in memory: `Epoch = 0`, `KeyCreatorFP = host`, host added to `Users`.
2. Hub emits `HubEventRoomCreated` → event consumer → `roomSvc.CreateRoom` (only persists if creator is a persistent user).
3. Hub immediately calls `broadcastRotation(room, emitEvent=true)`:
   - Sets `room.PendingRotation { Epoch: 0, KeyCreatorFP: host }`
   - Sends `room_key_rotation` WS to all online members (only the host at this point)
   - Emits `HubEventKeyRotationTriggered` → event consumer → `roomSvc.UpdateEpochAndKeyCreator` (DB sync)
4. The host is the sole member and key creator. It generates epoch 0 key, encrypts it to itself, and submits via `POST /api/rooms/:id/epoch-keys`.

---

### 4.2 Key Creator Selection Algorithm

Used in all rotation triggers. Implemented in `hub_handler.go: selectNextByLex`.

Priority order (lex-lowest fingerprint within each bucket, first non-empty bucket wins):

```
1. Online  + Persistent   ← preferred
2. Online  + Ephemeral
3. Offline + Persistent
4. Offline + Ephemeral    ← last resort
```

"Online" means the user has an active WebSocket connection in the hub at the time of selection. "Offline" means they are a known room member but not currently connected.

The current key creator is kept if they are online; selection only runs if the current creator is offline or being excluded (e.g., on disconnect).

---

### 4.3 Member Join — Epoch N+1

```
Client:  POST /api/rooms/:room_id/users/:user_fp
```

**DB-first** — hub is updated after DB is already consistent:

1. Handler verifies caller is the host.
2. Calls `roomSvc.AddMemberDirect` → inserts into `room_members` (skipped if room has no DB record — ephemeral-only room).
3. Runs key creator selection: keeps current creator if online; otherwise 4-tier lex selection.
4. Increments epoch, updates `rooms.current_epoch` and `rooms.key_creator_fp` in DB.
5. Sends `RoomMembersChangeRequest` to hub `JoinRoom` channel → `handleJoinRoom`:
   - Adds new member to `room.MemberPublicKeys` and `room.MemberModes` (regardless of online status).
   - If member is online, adds to `room.Users`.
   - Updates `room.Epoch` and `room.KeyCreatorFP`.
   - Calls `broadcastRotation(room, emitEvent=false)` — DB already updated, no event needed.
   - Calls `broadcastMemberJoined` → sends `join_room` WS to all online members.

If the room is not in the hub (all members offline), the hub sync is silently skipped — the DB is already correct.

---

### 4.4 Member Remove — Epoch N+1

```
Client:  DELETE /api/rooms/:room_id/users/:user_fp
```

**DB-first:**

**Special case — last member:**
- Room is deleted from DB (members, messages, key slots).
- Hub removes room from memory. No rotation.

**Normal case:**
1. Handler verifies caller is the host.
2. Removes member from `room_members` in DB.
3. If removed member was the host: selects new host via 4-tier lex (excluding removed member).
4. Selects new key creator via 4-tier lex (excluding removed member).
5. Increments epoch, updates DB.
6. Sends `RemoveFromRoomRequest` to hub → `handleRemoveFromRoom`:
   - Removes member from `room.Users`, `room.MemberPublicKeys`, `room.MemberModes`.
   - Updates `room.HostFP` if host changed.
   - Updates `room.Epoch` and `room.KeyCreatorFP`.
   - If no online members remain: room goes offline (removed from hub, stays in DB).
   - Otherwise: `broadcastRotation(room, emitEvent=false)` + `broadcastMemberLeft` → `leave_room` WS.

---

### 4.5 Epoch Key Submission and Delivery

```
Client (key creator):  POST /api/rooms/:room_id/epoch-keys
Body: { epoch: int64, keys: [{ recipient_fp, encrypted_key (base64) }] }
```

**Validations (against DB state):**
- Caller must be `rooms.key_creator_fp`.
- `req.Epoch` must equal `rooms.current_epoch`.
- No key slots may already exist for this epoch in `room_key_slots`.

**Processing:**
1. Decodes each `encrypted_key` from base64.
2. Calls `roomSvc.StoreKeySlots` → inserts into `room_key_slots`.
   - **Ephemeral recipients are silently skipped** — they have no row in `users`, so the FK constraint would fail. Their key is delivered in-memory only (next step).
3. Sends `EpochKeysSubmittedRequest` to hub → `handleEpochKeysSubmitted`:
   - Clears `room.PendingRotation`.
   - For each entry: if recipient is **online**, sends `room_key_slot` WS directly.
   - Offline persistent recipients: key is in DB — delivered automatically on next WS connect (see 4.8).
   - Offline ephemeral recipients: key is not stored and cannot be delivered — they must re-join the room.

```json
{
  "type": "room_key_slot",
  "payload": {
    "room_id":       "string",
    "epoch":         int64,
    "encrypted_key": "string (base64)"
  }
}
```

---

### 4.6 Key Creator Disconnects During Pending Rotation

When a user disconnects (`handleUnregisterUser`), the hub checks every room they were in:

```
if room.PendingRotation != nil && room.PendingRotation.KeyCreatorFP == user.Fingerprint:
    newCreator = selectNextByLex(room, excludeFP=user.Fingerprint)
    room.KeyCreatorFP = newCreator
    broadcastRotation(room, emitEvent=true)   ← same epoch, new creator
```

`emitEvent=true` → `HubEventKeyRotationTriggered` → event consumer → `roomSvc.UpdateEpochAndKeyCreator` (DB sync).

The epoch does **not** increment. The same epoch is redistributed to a new key creator. All online members receive a new `room_key_rotation` event with the updated `key_creator_fp`.

---

### 4.7 Key Creator Reconnects Mid-Rotation

When a persistent user reconnects (WS connect), the handler slots them into their hub rooms via `AddToRoom`. In `handleAddToRoom`:

```
if room.PendingRotation != nil && room.PendingRotation.KeyCreatorFP == user.Fingerprint:
    sendRotationToUser(user, room)   ← unicast only to this user
```

No DB change. No event. The rotation is already in progress — the user just missed their WS message and needs it resent.

---

### 4.8 Persistent User Reconnect

On WS connect, two actions happen automatically for persistent users:

**Room state sync:**

```go
for _, dbRoom := range GetUserRooms(fingerprint) {
    if room is in hub:
        AddToRoom ← hub                          // slot user in; resend rotation if they're key creator
    else if dbRoom.KeyCreatorFP == this user:
        hasPending = !HasKeySlots(roomID, epoch) // no slots yet = rotation pending
        RestoreRoom ← hub { HasPendingRotation: hasPending }
        // handleRestoreRoom: if pending rotation and key creator online → broadcastRotation
}
```

**Key slot delivery:**

```go
slots = GetKeySlotsByRecipient(fingerprint)
for each slot:
    send room_key_slot WS
```

All stored key slots across all rooms and epochs are pushed immediately on connect. The client receives every key it missed while offline — no polling or explicit API call needed.

---

### 4.9 Message to Offline Room

If a user sends a message to a room that is not currently in the hub:

1. `handleInboundMessage` detects room not in `h.Rooms` → emits `HubEventRoomOffline`.
2. Event consumer: calls `GetRoomByID` + `GetMembersWithPublicKeys`, verifies sender is a member.
3. Sends `LoadRoomAndDeliver` to hub → `handleLoadRoomAndDeliver`:
   - Creates room entry in hub with DB state.
   - Routes the pending message to all online members.
   - Emits `HubEventMessageCreated` for DB persistence.

---

### 4.10 Epoch Mismatch Rejection

If a sender's message epoch does not match the room's current epoch:

```
if message.Epoch != room.Epoch:
    send epoch_mismatch WS back to sender
    drop message
```

```json
{
  "type": "epoch_mismatch",
  "payload": {
    "room_id":       "string",
    "current_epoch": int64
  }
}
```

The sender must wait for a valid `room_key_slot` for the current epoch before retrying.

---

## 5. Why One Key Pair Is Sufficient

| Concern                                 | Handled By                                           |
|------------------------------------------|------------------------------------------------------|
| Gate authentication (signature)          | Ed25519 sign/verify                                  |
| User identity / routing                  | SHA-256 fingerprint of Ed25519 public key            |
| Room key distribution                    | Encrypt room key to each member's X25519 public key |
| Message encryption                       | Symmetric AEAD with room key                         |
| Message signing / authenticity           | Ed25519 signature on messages                        |

A second key pair is unnecessary. The Ed25519 ↔ X25519 conversion provides both signing and encryption from a single generated key.

---

## 6. Algorithm Choice Rationale

**Curve25519 (Ed25519 + X25519)** is chosen over NIST P-256 (ECDSA + ECDH) for the following reasons:

- Constant-time by design — fewer side-channel risks
- Simpler implementation — smaller attack surface
- Faster on all platforms including low-power devices
- No reliance on external random number generators during signing (Ed25519 is deterministic)
- Broadly supported: Signal Protocol, WireGuard, libsodium, Web Crypto API
- Well-suited for a lightweight, self-hosted system targeting minimal dependencies

---

## 7. What the Server Knows vs. Does Not Know

| Server Knows                              | Server Does Not Know                     |
|--------------------------------------------|------------------------------------------|
| Public keys and fingerprints               | Private keys                             |
| Which users are connected                  | Room keys (any epoch)                    |
| Which users are in which room              | Plaintext message content                |
| Encrypted ciphertext (stored)              | Any decryption capability                |
| Epoch numbers and key creator assignments  | The actual key material                  |
| Connection IDs and session IDs             | Contents of key distribution messages    |

The server is a **transport, routing, coordination, and storage layer**. It cannot decrypt any user content.

---

## 8. Key Lifecycle

### Ephemeral Mode
- Ed25519 key pair generated in memory on page load.
- Keys destroyed on tab close or page refresh.
- Room keys and message history are lost on reload.
- Key pair and room keys are not persisted on the client.
- User record is not saved to the server database.

### Persistent Mode
- Ed25519 key pair generated once and stored in the browser (IndexedDB or localStorage).
- Private key optionally encrypted with a user-chosen passphrase.
- Room keys stored per `(room_id, epoch)` in the local key store (IndexedDB), optionally encrypted.
- Identity and message history persist across sessions.
- Server stores encrypted messages but never private or room keys.

---

## 9. Summary

The system uses a single Ed25519 key pair per user. Ed25519 handles all signing and authentication. X25519 (derived from the same key) handles encrypting room keys to individual members. All rooms — regardless of member count — use the same symmetric key distribution model with epoch-based key rotation. The server stores encrypted messages and coordinates key rotation events but cannot decrypt any content. Clients maintain a local key store of epoch keys to decrypt both live and historical messages.

---

## 10. Transport & Operational Security

This section documents server-side protections that operate at the transport, authentication, and session layer — separate from end-to-end encryption.

### 10.1 Replay Attack Prevention

Both the HTTP and WebSocket authentication layers include a two-layer replay defence:

**Timestamp window:** Every signed request includes a Unix timestamp. The server rejects requests where the timestamp falls outside a ±30-second window of the current server time.

**Nonce store (`internal/utils/nonce_store.go`):** Each accepted Ed25519 signature is recorded in an in-memory store with a 60-second TTL. A second request carrying the same signature is rejected with `401 replayed request`, even if the timestamp is still within the window. The store runs a background cleanup goroutine that evicts expired entries every 60 seconds.

This applies to:
- HTTP protected routes — checked in `AuthMiddleware` (`internal/api/middleware.go`)
- WebSocket auth handshake — checked in `ws.go` before the connection is promoted

### 10.2 WebSocket Origin Restriction

The WebSocket upgrader (`internal/api/handlers/ws.go`) validates the `Origin` header on browser connections:

```go
CheckOrigin: func(r *http.Request) bool {
    origin := r.Header.Get("Origin")
    if origin == "" {
        return true // non-browser clients (e.g. native apps, curl)
    }
    u, _ := url.Parse(origin)
    return u.Host == r.Host // same host only
}
```

Cross-origin WebSocket connections from other domains are rejected. This prevents a malicious website from silently opening a socket to a running server while the user is authenticated in another tab.

### 10.3 Rate Limiting

Three independent rate limiters are in effect:

| Layer | Scope | Limit | Location |
|-------|-------|-------|----------|
| Global | All routes | 100 req/s across all clients | `routes.go`, Echo middleware |
| Gate endpoint | Per client IP | 10 req/min (burst 10) | `routes.go`, `POST /api/auth/gate` only |
| WebSocket messages | Per connected user | 5 msg/s (burst 10) | `hub/user.go`, `ReadPump` |

The per-IP gate limiter uses Echo's `RateLimiterWithConfig` with `c.RealIP()` as the identifier. Requests exceeding the limit receive `429 Too Many Requests`. This prevents brute-forcing the 64-character hex gate token.

The per-user message limiter (`rate.NewLimiter(1, 5)` from `golang.org/x/time/rate`) is enforced in each user's `ReadPump` goroutine. A user may send up to 5 messages in a quick burst; after that the sustained rate is capped at 1 message/second. Messages that exceed the limit are dropped and logged at WARN level; the WebSocket connection itself is not closed.

### 10.4 Session Expiry

Sessions are stored in an in-memory `session.Store` (`internal/session/store.go`) with a 24-hour TTL. The store tracks creation time for each session and:

- Returns `false` from `Get` immediately if the session has exceeded the TTL
- Runs a background goroutine (every hour) that evicts expired sessions and releases their associated username

Sessions are also removed explicitly on user disconnect (`ws.go` cleanup path) and on ephemeral user disconnect. The 24-hour TTL acts as a safety net for sessions that were never explicitly closed (e.g. network failure, browser crash).

### 10.5 Input Length Limits

User-supplied string inputs are validated at the handler layer before any service or database call:

| Field | Limit | Location |
|-------|-------|----------|
| Username | ≤ 50 characters | `handlers/gate.go` |
| Room name | 1–100 characters | `handlers/NewRoom.go` |
| Pagination `offset` | ≤ 1,000,000 | `handlers/getRoomMessages.go`, `handlers/getKeySlots.go` |

### 10.6 Atomic Gate Token Use Count

The gate token use counter (`gate_tokens.use_count`) is incremented using a single SQL statement with `RETURNING`:

```sql
UPDATE gate_tokens SET use_count = use_count + 1 WHERE id = ? RETURNING use_count
```

This replaces the prior two-query approach (UPDATE then SELECT), which had a race window where concurrent requests could read the same stale count and bypass the max-uses limit.

### 10.7 Schema Migration Safety

The `addColumnIfNotExists` and `dropColumnIfExists` migration helpers build `ALTER TABLE` statements using `fmt.Sprintf` with caller-supplied table and column names (SQLite does not support parameterized schema identifiers). An allowlist (`validMigrationTargets` in `internal/db/migrations.go`) enumerates every permitted table/column pair. Any call with a name outside the allowlist returns an error before any SQL is constructed.