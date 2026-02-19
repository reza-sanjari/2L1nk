package api

import (
	"2L1nk/internal/api/handlers"

	"github.com/labstack/echo/v4"
)

func RegisterRoutes(e *echo.Echo, h *handlers.Handler) {
	api := e.Group("/api")

	api.GET("/health", h.Health)
}
