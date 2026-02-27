package models

type UserMode int

const (
	UserModeEphemeral UserMode = iota
	UserModePersistent
)
