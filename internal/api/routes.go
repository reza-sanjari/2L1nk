package api

import (
	"2L1nk/internal/api/handlers"
	"2L1nk/internal/session"

	"github.com/labstack/echo/v4"
)

func RegisterRoutes(e *echo.Echo, h *handlers.Handler, store *session.Store) {
	api := e.Group("/api")

	api.GET("/health", h.Health)

	protected := api.Group("", SessionAuthMiddleware(store))
	_ = protected
}
