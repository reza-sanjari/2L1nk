package hub

import "2L1nk/internal/models"

type HubEventType string

const (
	HubEventRoomCreated     HubEventType = "room_created"
	HubEventMemberJoined    HubEventType = "member_joined"
	HubEventMessageCreated  HubEventType = "message_created"
	HubEventRoomOffline     HubEventType = "room_offline"
	HubEventMemberRemoved   HubEventType = "member_removed"
	HubEventRoomDeleted     HubEventType = "room_deleted"
	HubEventHostTransferred HubEventType = "host_transferred"
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

type RoomOfflinePayload struct {
	RoomID   string
	SenderFP string
	Message  WSMessageEnvelope
}

type MemberRemovedPayload struct {
	RoomID   string
	MemberFP string
}

type RoomDeletedPayload struct {
	RoomID string
}

type HostTransferredPayload struct {
	RoomID    string
	NewHostFP string
}
