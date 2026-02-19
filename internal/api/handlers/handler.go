package handlers

import (
	"2L1nk/internal/service"
)

// Handler is the single entry point for all HTTP handlers.
// It holds the service container and the hub.
type Handler struct {
	Services *service.Container
	// Hub *hub.Hub   // uncomment when hub is implemented
}

// NewHandler creates a Handler with the full service container and hub.
func NewHandler(services *service.Container) *Handler {
	return &Handler{
		Services: services,
	}
}
