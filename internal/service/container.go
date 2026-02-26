package service

// Container bundles all application services.
// Handlers receive this instead of individual services.
type Container struct {
	Health *HealthService
	Gate   *GateService
}

// NewContainer constructs the service container with all services wired.
func NewContainer(health *HealthService, Gate *GateService) *Container {
	return &Container{
		Health: health,
		Gate:   Gate,
	}
}
