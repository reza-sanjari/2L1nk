package service

// Container bundles all application services.
// Handlers receive this instead of individual services.
type Container struct {
	Health *HealthService
	// Future services go here:
	// User    *UserService
	// Message *MessageService
}

// NewContainer constructs the service container with all services wired.
func NewContainer(health *HealthService) *Container {
	return &Container{
		Health: health,
	}
}
