package api

import (
	"2L1nk/internal/api/handlers"
	"2L1nk/internal/logger"
	"2L1nk/internal/session"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func RegisterRoutes(e *echo.Echo, h *handlers.Handler, store *session.Store) {
	e.HideBanner = true

	//Logger middleware with costume config
	e.Use(middleware.RequestLoggerWithConfig(logger.MinimalLoggerConfig()))

	api := e.Group("/api")

	api.GET("/health", h.Health)

	protected := api.Group("", SessionAuthMiddleware(store))
	_ = protected
}
