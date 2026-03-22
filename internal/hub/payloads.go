package hub

import (
	"2L1nk/internal/models"
	"encoding/json"
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
