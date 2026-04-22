# SQLite Database Schema

All timestamps are stored as Unix epoch integers (seconds).
IDs are UUIDs stored as TEXT unless noted otherwise. The `users` table uses `fingerprint` as its primary key — there is no separate id column.
Only persistent data lives here — runtime state (sessions, active WebSocket connections, Pion peer connections) is never stored.

---

## Table: `users`

Stores every user identity, persistent and ephemeral. The `mode` column distinguishes the two. Ephemeral users are stored in the DB just like persistent ones — the only difference is that their private key never leaves the client, so a browser refresh means a new Ed25519 keypair and therefore a new fingerprint (the old row is left behind).

| Column              | Type    | Constraints          | Description                                                                 |
|---------------------|---------|----------------------|-----------------------------------------------------------------------------|
| `fingerprint`       | TEXT    | PRIMARY KEY          | SHA-256 of Ed25519 public key — canonical user ID used for routing and room membership |
| `public_key`        | TEXT    | NOT NULL             | Base64-encoded Ed25519 public key                                           |
| `x25519_public_key` | TEXT    | NOT NULL DEFAULT ''  | Base64-encoded X25519 public key (converted from Ed25519) — used for encrypting room keys to this user |
| `username`          | TEXT    |                      | Optional display name — not used for auth                                   |
| `mode`              | INTEGER | NOT NULL DEFAULT 1   | 0 = ephemeral (no client-side key storage, messages not persisted), 1 = persistent |
| `created_at`        | INTEGER | NOT NULL             | Unix timestamp of first registration                                        |

---

## Table: `rooms`

Stores room metadata. There is no DM type — a two-person room is just a room with two members. All rooms are structurally identical.

| Column           | Type    | Constraints        | Description                                                                                          |
|------------------|---------|--------------------|------------------------------------------------------------------------------------------------------|
| `id`             | TEXT    | PRIMARY KEY        | Room UUID — used as the routing key for both messages and voice sessions                             |
| `name`           | TEXT    |                    | Display name of the room                                                                             |
| `current_epoch`  | INTEGER | NOT NULL DEFAULT 0 | Current key epoch — increments on every membership change and triggers key rotation                  |
| `host_fp`        | TEXT    |                    | Fingerprint of the room host (admin who can add/remove members). No FK — historical (`migrateRoomsDropKeyCreatorFK`); re-adding would require a destructive migration |
| `key_creator_fp` | TEXT    |                    | Fingerprint of the member responsible for generating the next epoch key. No FK, same historical reason. Starts as the host; reassigned by lex priority (online persistent → online ephemeral → offline persistent → offline ephemeral) if the host/current creator leaves or is offline during a rotation |
| `created_at`     | INTEGER | NOT NULL           | Unix timestamp of room creation                                                                      |

---

## Table: `room_members`

Junction table tracking which users belong to which rooms. Persistent and ephemeral users are both tracked here — the FK to `users(fingerprint)` works for both because every user has a row in `users`.

| Column      | Type    | Constraints                           | Description                              |
|-------------|---------|---------------------------------------|------------------------------------------|
| `room_id`   | TEXT    | NOT NULL, REFERENCES rooms(id)        | FK to the room                           |
| `member_fp` | TEXT    | NOT NULL, REFERENCES users(fingerprint) | FK to the user via their fingerprint   |
| `joined_at` | INTEGER | NOT NULL                              | Unix timestamp of when the member joined |

**Primary Key:** (`room_id`, `member_fp`)

---

## Table: `messages`

Stores encrypted message blobs. The server cannot read any payload — it is a dumb storage layer.

| Column       | Type    | Constraints                    | Description                                                                                           |
|--------------|---------|--------------------------------|-------------------------------------------------------------------------------------------------------|
| `id`         | TEXT    | PRIMARY KEY                    | Message UUID                                                                                          |
| `room_id`    | TEXT    | NOT NULL, REFERENCES rooms(id) | Room the message belongs to                                                                           |
| `sender_fp`  | TEXT    | NOT NULL                       | Fingerprint of the sender. No FK — messages from ephemeral senders are never written (see `MessageService.ProcessMessage`), so in practice every persisted row points at a user that exists, but the column is left un-FK'd for schema resilience |
| `epoch`      | INTEGER | NOT NULL                       | Key epoch under which the message was encrypted — clients use this to look up the correct decryption key from their local key store |
| `type`       | INTEGER | NOT NULL                       | 0 = text, 1 = system, 2 = signal/WebRTC signaling                                                    |
| `ciphertext` | BLOB    | NOT NULL                       | Encrypted payload (AES-256-GCM or ChaCha20-Poly1305) — stored as a base64-encoded string; opaque to the server |
| `created_at` | INTEGER | NOT NULL                       | Unix timestamp of message receipt                                                                     |

---

## Table: `room_key_slots`

Stores the per-member encrypted copies of each epoch's room key. When a client reconnects the server re-delivers their slot so they can decrypt stored messages without the server ever seeing the plaintext key.

| Column          | Type    | Constraints                             | Description                                                                              |
|-----------------|---------|-----------------------------------------|------------------------------------------------------------------------------------------|
| `room_id`       | TEXT    | NOT NULL, REFERENCES rooms(id)          | FK to the room                                                                           |
| `epoch`         | INTEGER | NOT NULL                                | The epoch this key slot belongs to                                                       |
| `recipient_fp`  | TEXT    | NOT NULL, REFERENCES users(fingerprint) | The member this encrypted key is intended for. Slots are stored for every member, persistent or ephemeral; the FK holds because all users are in `users`. Ephemeral members just can't decrypt after a browser refresh — their private key is gone |
| `encrypted_key` | BLOB    | NOT NULL                                | The symmetric room key encrypted with the recipient's X25519 public key — opaque to server |
| `created_at`    | INTEGER | NOT NULL                                | Unix timestamp of when this slot was written                                             |

**Primary Key:** (`room_id`, `epoch`, `recipient_fp`)

---

## Table: `voice_sessions`

Records voice sessions that have occurred or are currently active within a room. One active session per room at a time (`ended_at IS NULL`). The Pion SFU peer connections themselves are runtime-only and never stored here.

| Column       | Type    | Constraints                    | Description                                                                    |
|--------------|---------|--------------------------------|--------------------------------------------------------------------------------|
| `id`         | TEXT    | PRIMARY KEY                    | Session UUID                                                                   |
| `room_id`    | TEXT    | NOT NULL, REFERENCES rooms(id) | The room this voice session belongs to — voice and text share the same room ID |
| `started_at` | INTEGER | NOT NULL                       | Unix timestamp of when the session was initiated                               |
| `ended_at`   | INTEGER |                                | Unix timestamp of when the session ended — NULL means the session is still active |

---

## Table: `voice_participants`

Tracks which users joined and left each voice session. Used to re-sync call state for reconnecting clients and to provide participant history.

| Column       | Type    | Constraints                                   | Description                                                           |
|--------------|---------|-----------------------------------------------|-----------------------------------------------------------------------|
| `session_id` | TEXT    | NOT NULL, REFERENCES voice_sessions(id)       | FK to the voice session                                               |
| `member_fp`  | TEXT    | NOT NULL                                      | Fingerprint of the participant. No FK — historical; every user now has a row in `users`, so an FK would be valid, but re-adding it would require a destructive migration |
| `joined_at`  | INTEGER | NOT NULL                                      | Unix timestamp of when this participant joined the session            |
| `left_at`    | INTEGER |                                               | Unix timestamp of when this participant left — NULL means still active |
| `muted`      | INTEGER | NOT NULL DEFAULT 0                            | Current mute state: 0 = unmuted, 1 = muted — updated in place on mute toggle |

**Primary Key:** (`session_id`, `member_fp`)

---

## Table: `gate_tokens`

Schema exists and is created on startup, but the table is **not currently used** by the application. Gate tokens are generated and validated entirely in memory (`internal/gate/gate.go`). Reserved for future persistence of gate tokens (e.g. surviving restarts).

| Column       | Type    | Constraints               | Description                                       |
|--------------|---------|---------------------------|---------------------------------------------------|
| `id`         | INTEGER | PRIMARY KEY AUTOINCREMENT | Internal row ID                                   |
| `token`      | TEXT    | UNIQUE NOT NULL           | The gate access token (random secret string)      |
| `created_at` | INTEGER | NOT NULL                  | Unix timestamp of when the token was generated    |

---

## Relationship Summary

```
users ──────────────────────────────────────────────────────┐
  │ fingerprint                                              │
  │                                                         │
  ├─< room_members >──── rooms                              │
  │     member_fp           │ id                            │
  │                         │                               │
  │                         ├─< messages                    │
  │                         │     room_id                   │
  │                         │     sender_fp ────────────────┘ (no FK, historical)
  │                         │     epoch
  │                         │
  │                         ├─< room_key_slots
  │                         │     room_id
  │                         │     epoch
  │                         │     recipient_fp (FK → users.fingerprint)
  │                         │
  │                         └─< voice_sessions
  │                               id
  │                               └─< voice_participants
  │                                     session_id
  └──────────────────────────────────  member_fp (no FK, historical)
```

---

## Design Notes

- **No room type column.** A DM is a room with 2 members. A group chat is a room with N members. The member count is the only structural difference. No type discrimination anywhere in the schema.
- **Voice sessions attach to the existing room.** Text and voice share the same `room_id`. There are no separate voice rooms. This directly aligns with the SFU design where voice is just an optional session within a room.
- **`voice_participants.muted` is updated in place.** Mute toggles are frequent and don't need history — a single column updated on each toggle is sufficient. The row is created on join and `left_at` is filled on leave.
- **Pion SFU peer connections are never persisted.** `voice_sessions` and `voice_participants` contain only signaling-level coordination metadata. The actual `webrtc.PeerConnection` objects live purely in the Hub's runtime memory.
- **`sender_fp` in `messages` and `member_fp` in `voice_participants` have no FK constraint.** This is historical: it dates back to when ephemeral users were not written to `users`. All users are now persisted (the `users.mode` column distinguishes ephemeral from persistent), so an FK would now be valid — but re-adding one requires a destructive table rebuild and isn't worth it.
- **`room_key_slots` is the server's reconnect re-delivery mechanism.** Zero plaintext keys ever stored — only ciphertexts that only the intended recipient can open with their X25519 private key. Old epoch slots are retained so clients can decrypt historical messages.
- **`current_epoch` in `rooms` drives key rotation.** Any member join or leave increments it in DB first, triggers a rotation WS broadcast, and updates `key_creator_fp` using lex priority (online persistent → online ephemeral → offline persistent → offline ephemeral). The hub is updated after DB as a cache.
- **`host_fp` and `key_creator_fp` are separate.** The host is the room admin; the key creator generates epoch keys. They start as the same person and are reassigned independently using the same lex method when either leaves.
- **`gate_tokens` is unused at runtime.** The table is created by migrations but the application manages gate tokens entirely in memory. No rows are written or read by the current implementation.
- **Nothing in this schema can decrypt any message or media stream.** The server is a transport, routing, coordination, and storage layer only.
