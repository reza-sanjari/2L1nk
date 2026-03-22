package service

// Container bundles all application services.
// Handlers receive this instead of individual services.
type Container struct {
	Health  *HealthService
	Gate    *GateService
	Room    *RoomService
	Message *MessageService
}

// NewContainer constructs the service container with all services wired.
func NewContainer(health *HealthService, gate *GateService, room *RoomService, message *MessageService) *Container {
	return &Container{
		Health:  health,
		Gate:    gate,
		Room:    room,
		Message: message,
	}
}
