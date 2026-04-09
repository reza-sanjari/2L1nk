package handlers

import (
	"2L1nk/internal/hub"
	"2L1nk/internal/logger"
	"2L1nk/internal/service"
	"2L1nk/internal/session"
	"2L1nk/internal/utils"
)

// Handler is the single entry point for all HTTP handlers.
// It holds the service container and the hub.
type Handler struct {
	services   *service.Container
	hub        *hub.Hub
	session    *session.Store
	logg       *logger.Logger
	nonceStore *utils.NonceStore
}

// NewHandler creates a Handler with the full service container and hub.
func NewHandler(services *service.Container, hub *hub.Hub, session *session.Store, logg *logger.Logger, ns *utils.NonceStore) *Handler {
	return &Handler{
		services:   services,
		hub:        hub,
		session:    session,
		logg:       logg,
		nonceStore: ns,
	}
}
