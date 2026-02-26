package api

import (
	"2L1nk/internal/api/handlers"
	"2L1nk/internal/logger"
	"2L1nk/internal/session"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func RegisterRoutes(e *echo.Echo, h *handlers.Handler, store *session.Store) {
	e.HideBanner = true

	// Security middleware
	e.Use(middleware.RequestID())
	e.Use(middleware.Secure())
	e.Use(middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(100)))
	e.Use(middleware.ContextTimeout(10 * time.Second))

	//Logger middleware with costume config
	e.Use(middleware.RequestLoggerWithConfig(logger.MinimalLoggerConfig()))

	api := e.Group("/api")

	api.GET("/health", h.Health)
	api.POST("/auth/gate", h.GateAuthorize)

	protected := api.Group("", SessionAuthMiddleware(store))
	_ = protected
}
