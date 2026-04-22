package hub

import (
	"2L1nk/internal/models"
	"encoding/json"
	"time"
)

type BroadcastRequest struct {
	Room     *Room
	Envelope WSMessageEnvelope
}

type WSMessageEnvelope struct {
	Sender  *User              `json:"-"` // server-only
	Type    models.WSEventType `json:"type"`
	Payload json.RawMessage    `json:"payload"`
}

// outboundEnvelope is what gets sent to clients — includes sender_fp.
type outboundEnvelope struct {
	SenderFP string             `json:"sender_fp"`
	Type     models.WSEventType `json:"type"`
	Payload  json.RawMessage    `json:"payload"`
}

// MessagePayload todo: chance ciphertext type to []byte
type MessagePayload struct {
	RoomID     string `json:"room_id"`
	Epoch      uint64 `json:"epoch"`
	Ciphertext string `json:"ciphertext"`
}

// MemberJoinedPayload is the WS payload sent to all room members when a new member joins.
type MemberJoinedPayload struct {
	RoomID string          `json:"room_id"`
	FP     string          `json:"fingerprint"`
	Mode   models.UserMode `json:"mode"`
}

// MemberLeftPayload is the WS payload sent to all room members when a member is removed.
type MemberLeftPayload struct {
	RoomID string `json:"room_id"`
	FP     string `json:"fingerprint"`
}

type AddToRoomRequest struct {
	RoomID string
	User   *User
}

// MemberKeyInfo carries a member's fingerprint, X25519 public key, and mode.
// Used when restoring rooms and in key rotation events.
type MemberKeyInfo struct {
	FP              string
	X25519PublicKey string
	Mode            models.UserMode
}

type RestoreRoomRequest struct {
	RoomID             string
	RoomName           string
	HostFP             string
	KeyCreatorFP       string
	Epoch              int64
	Members            []MemberKeyInfo // hub adds online members; all carry their X25519 key
	HasPendingRotation bool            // true if current epoch has no key slots yet
}

type RemoveFromRoomRequest struct {
	RoomID          string
	MemberFP        string
	Deleted         bool // true if room was deleted from DB — hub just removes from h.Rooms
	NewEpoch        int64
	NewKeyCreatorFP string
	NewHostFP       string // "" if host is unchanged
}

// MessagesPurgedPayload is sent to room members when a user purges all their messages.
type MessagesPurgedPayload struct {
	SenderFP string `json:"sender_fp"`
	RoomID   string `json:"room_id"`
}

// PurgeRequest asks the hub to broadcast a messages_purged event to all active rooms the user is a member of.
type PurgeRequest struct {
	SenderFP string
}

type LoadRoomAndDeliverRequest struct {
	RoomID   string
	RoomName string
	HostFP   string
	Epoch    int64
	Members  []MemberKeyInfo
	Message  WSMessageEnvelope
}

type SendErrorRequest struct {
	UserFP  string
	Message string
}

// PendingRotation tracks an in-progress key rotation for a room.
type PendingRotation struct {
	Epoch        int64
	KeyCreatorFP string
	TriggeredAt  time.Time
}

// MemberWithKey is a member entry sent in a RoomKeyRotationPayload.
type MemberWithKey struct {
	Fingerprint     string `json:"fingerprint"`
	X25519PublicKey string `json:"x25519_public_key"`
}

// RoomKeyRotationPayload is the WS payload for a room_key_rotation event.
type RoomKeyRotationPayload struct {
	RoomID       string          `json:"room_id"`
	Epoch        int64           `json:"epoch"`
	KeyCreatorFP string          `json:"key_creator_fp"`
	Members      []MemberWithKey `json:"members"`
}

// RoomKeySlotPayload is the WS payload delivering an encrypted key to one member.
type RoomKeySlotPayload struct {
	RoomID       string `json:"room_id"`
	Epoch        int64  `json:"epoch"`
	EncryptedKey string `json:"encrypted_key"` // base64-encoded
}

// EpochMismatchPayload is sent back to a sender when their message epoch is stale.
type EpochMismatchPayload struct {
	RoomID       string `json:"room_id"`
	CurrentEpoch int64  `json:"current_epoch"`
}

// KeySlotEntry is one encrypted key entry inside EpochKeysSubmittedRequest.
type KeySlotEntry struct {
	RecipientFP  string `json:"recipient_fp"`
	EncryptedKey []byte // decoded from base64 at the HTTP handler
}

// SignalPayloadInbound is the payload of a "signal" WS message from a client.
type SignalPayloadInbound struct {
	RoomID   string          `json:"room_id"`
	TargetFP string          `json:"target_fp"`
	Signal   json.RawMessage `json:"signal"` // opaque: offer, answer, or ice candidate
}

// SignalPayloadOutbound is the payload forwarded to the signal target.
// FromFP is injected by the server so the recipient knows who sent the signal.
type SignalPayloadOutbound struct {
	RoomID string          `json:"room_id"`
	FromFP string          `json:"from_fp"`
	Signal json.RawMessage `json:"signal"`
}

// VoiceJoinedPayloadInbound is the payload of a "voice_joined" message from a client.
type VoiceJoinedPayloadInbound struct {
	RoomID string `json:"room_id"`
}

// VoiceJoinedPayloadOutbound is sent back to the joiner (VoiceUsers populated) and
// broadcast to all other room members (VoiceUsers omitted).
type VoiceJoinedPayloadOutbound struct {
	RoomID      string   `json:"room_id"`
	Fingerprint string   `json:"fingerprint"`
	VoiceUsers  []string `json:"voice_users,omitempty"`
}

// VoiceLeftPayloadInbound is the payload of a "voice_left" message from a client.
type VoiceLeftPayloadInbound struct {
	RoomID string `json:"room_id"`
}

// VoiceLeftPayloadOutbound is broadcast to all room members when someone leaves voice.
type VoiceLeftPayloadOutbound struct {
	RoomID      string `json:"room_id"`
	Fingerprint string `json:"fingerprint"`
}

// EpochKeysSubmittedRequest is sent to the hub after the key creator POSTs epoch keys.
type EpochKeysSubmittedRequest struct {
	RoomID string
	Epoch  int64
	Keys   []KeySlotEntry
}

// RoomUpdatedPayload is the WS payload for a room_updated event.
// Structure mirrors the per-room shape returned by GET /rooms so the frontend
// can update its state without a separate REST call.
type RoomUpdatedPayload struct {
	RoomID string           `json:"room_id"`
	Name   string           `json:"name"`
	Epoch  int64            `json:"epoch"`
	Online bool             `json:"online"`
	Host   *RoomMemberInfo  `json:"host,omitempty"`
	Users  []RoomMemberInfo `json:"users,omitempty"`
}

// BroadcastToRoomRequest asks the hub to send pre-marshaled data to all online
// members of a room. The caller is responsible for marshaling the full WS envelope.
type BroadcastToRoomRequest struct {
	RoomID string
	Data   []byte
}
