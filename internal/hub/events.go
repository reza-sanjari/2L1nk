package hub

import "2L1nk/internal/models"

type HubEventType string

const (
	HubEventRoomCreated    HubEventType = "room_created"
	HubEventMemberJoined   HubEventType = "member_joined"
	HubEventMessageCreated HubEventType = "message_created"
)

type HubEvent struct {
	Type    HubEventType
	Payload any
}

type RoomCreatedPayload struct {
	RoomID      string
	Name        string
	CreatorFP   string
	CreatorMode models.UserMode
	CreatedAt   int64
}

type MemberJoinedPayload struct {
	RoomID     string
	MemberFP   string
	MemberMode models.UserMode
	JoinedAt   int64
}

type MessageCreatedPayload struct {
	ID         string
	RoomID     string
	SenderFP   string
	Epoch      int64
	Ciphertext string
	CreatedAt  int64
}
