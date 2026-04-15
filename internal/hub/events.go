package hub

import "2L1nk/internal/models"

type HubEventType string

const (
	HubEventRoomCreated          HubEventType = "room_created"
	HubEventMessageCreated       HubEventType = "message_created"
	HubEventRoomOffline          HubEventType = "room_offline"
	HubEventKeyRotationTriggered HubEventType = "key_rotation_triggered"
	HubEventEpochKeysSubmitted   HubEventType = "epoch_keys_submitted"
	HubEventHostTransferred      HubEventType = "host_transferred"
	HubEventRoomUpdated          HubEventType = "room_updated"
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

type MessageCreatedPayload struct {
	ID         string
	RoomID     string
	SenderFP   string
	SenderMode models.UserMode
	Epoch      int64
	Ciphertext string
	CreatedAt  int64
}

type RoomOfflinePayload struct {
	RoomID   string
	SenderFP string
	Message  WSMessageEnvelope
}

type KeyRotationTriggeredPayload struct {
	RoomID       string
	Epoch        int64
	KeyCreatorFP string
}

type EpochKeysSubmittedPayload struct {
	RoomID string
	Epoch  int64
	Keys   []KeySlotEntry
}

type HostTransferredPayload struct {
	RoomID    string
	NewHostFP string // empty string = no eligible host
}

// RoomUpdatedEventPayload carries the room ID that the event consumer should
// fetch and broadcast as a room_updated WS event to all online members.
type RoomUpdatedEventPayload struct {
	RoomID string
}
