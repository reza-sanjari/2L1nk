package service

// Container bundles all application services.
// Handlers receive this instead of individual services.
type Container struct {
	Health *HealthService
	Gate   *GateService
	Room   *RoomService
}

// NewContainer constructs the service container with all services wired.
func NewContainer(health *HealthService, Gate *GateService, Room *RoomService) *Container {
	return &Container{
		Health: health,
		Gate:   Gate,
		Room:   Room,
	}
}
