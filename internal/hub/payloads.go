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

// MessagePayload todo: chance ciphertext type to []byte
type MessagePayload struct {
	RoomID     string `json:"room_id"`
	Epoch      uint64 `json:"epoch"`
	Ciphertext string `json:"ciphertext"`
}

type AddToRoomRequest struct {
	RoomID string
	User   *User
}

// MemberKeyInfo carries a member's fingerprint and X25519 public key.
// Used when restoring rooms and in key rotation events.
type MemberKeyInfo struct {
	FP              string
	X25519PublicKey string
}

type RestoreRoomRequest struct {
	RoomID   string
	RoomName string
	HostFP   string
	Epoch    int64
	Members  []MemberKeyInfo // hub adds online members; all carry their X25519 key
}

type RemoveFromRoomRequest struct {
	RoomID   string
	MemberFP string
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

// EpochKeysSubmittedRequest is sent to the hub after the key creator POSTs epoch keys.
type EpochKeysSubmittedRequest struct {
	RoomID string
	Epoch  int64
	Keys   []KeySlotEntry
}
