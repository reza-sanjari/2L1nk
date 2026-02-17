package api

import (
	"2L1nk/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

type Handler struct {
	Health *handlers.HealthHandler
}

func NewHandler(healthSvc handlers.HealthService) *Handler {
	return &Handler{
		Health: handlers.NewHealthHandler(healthSvc),
	}
}

func RegisterRoutes(e *echo.Echo, h *Handler) {
	api := e.Group("/api")

	api.GET("/health", h.Health.Health)
}
