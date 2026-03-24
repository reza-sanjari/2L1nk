package models

type UserMode int

const (
	UserModeEphemeral UserMode = iota
	UserModePersistent
)

type WSEventType string

const (
	Auth          WSEventType = "auth"
	Message       WSEventType = "message"
	JoinRoom      WSEventType = "join_room"
	LeaveRoom     WSEventType = "leave_room"
	Signal        WSEventType = "signal"
	Error         WSEventType = "error"
	KeyRotation   WSEventType = "room_key_rotation"
	KeySlot       WSEventType = "room_key_slot"
	EpochMismatch WSEventType = "epoch_mismatch"
)
