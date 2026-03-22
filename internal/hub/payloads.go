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

type RestoreRoomRequest struct {
	RoomID    string
	RoomName  string
	HostFP    string
	Epoch     int64
	MemberFPs []string // hub adds those currently in h.Users
}

type RemoveFromRoomRequest struct {
	RoomID   string
	MemberFP string
}

type LoadRoomAndDeliverRequest struct {
	RoomID    string
	RoomName  string
	HostFP    string
	Epoch     int64
	MemberFPs []string
	Message   WSMessageEnvelope
}

type SendErrorRequest struct {
	UserFP  string
	Message string
}
