package models

type UserMode int

const (
	UserModeEphemeral UserMode = iota
	UserModePersistent
)

func (m UserMode) String() string {
	if m == UserModePersistent {
		return "persistent"
	}
	return "ephemeral"
}

type WSEventType string

const (
	Auth          WSEventType = "auth"
	Message       WSEventType = "message"
	JoinRoom      WSEventType = "join_room"
	LeaveRoom     WSEventType = "leave_room"
	Signal        WSEventType = "signal"
	VoiceJoined   WSEventType = "voice_joined"
	VoiceLeft     WSEventType = "voice_left"
	Error         WSEventType = "error"
	KeyRotation       WSEventType = "room_key_rotation"
	KeyRotationUpdate WSEventType = "room_key_rotation_update"
	KeySlot           WSEventType = "room_key_slot"
	EpochMismatch     WSEventType = "epoch_mismatch"
	MessagesPurged    WSEventType = "messages_purged"
	RoomUpdated       WSEventType = "room_updated"
)
