package handlers

import (
	"2L1nk/internal/hub"
	"2L1nk/internal/logger"
	"2L1nk/internal/service"
	"2L1nk/internal/session"
)

// Handler is the single entry point for all HTTP handlers.
// It holds the service container and the hub.
type Handler struct {
	services *service.Container
	hub      *hub.Hub
	session  *session.Store
	logg     *logger.Logger
}

// NewHandler creates a Handler with the full service container and hub.
func NewHandler(services *service.Container, hub *hub.Hub, session *session.Store, logg *logger.Logger) *Handler {
	return &Handler{
		services: services,
		hub:      hub,
		session:  session,
		logg:     logg,
	}
}
