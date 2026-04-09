package api

import (
	"2L1nk/internal/api/handlers"
	"2L1nk/internal/session"
	"2L1nk/internal/utils"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"golang.org/x/time/rate"
)

func RegisterRoutes(e *echo.Echo, h *handlers.Handler, store *session.Store, ns *utils.NonceStore) {
	e.HideBanner = true
	e.HidePort = true

	// Security middleware
	e.Use(middleware.RequestID())
	e.Use(middleware.Secure())
	e.Use(middleware.RateLimiter(middleware.NewRateLimiterMemoryStore(100)))
	e.Use(middleware.ContextTimeout(10 * time.Second))

	//Logger middleware with costume config
	//e.Use(middleware.RequestLoggerWithConfig(logger.MinimalLoggerConfig()))

	api := e.Group("/api")

	api.GET("/health", h.Health)

	// Per-IP rate limiter on the gate endpoint: 10 attempts per minute to prevent brute force.
	gateIPLimiter := middleware.RateLimiterWithConfig(middleware.RateLimiterConfig{
		Store: middleware.NewRateLimiterMemoryStoreWithConfig(
			middleware.RateLimiterMemoryStoreConfig{
				Rate:      rate.Limit(10.0 / 60.0),
				Burst:     10,
				ExpiresIn: 5 * time.Minute,
			},
		),
		IdentifierExtractor: func(c echo.Context) (string, error) {
			return c.RealIP(), nil
		},
		DenyHandler: func(c echo.Context, identifier string, err error) error {
			return c.JSON(http.StatusTooManyRequests, map[string]string{"error": "too many requests"})
		},
	})
	api.POST("/auth/gate", h.GateAuthorize, gateIPLimiter)

	api.GET("/ws", h.Ws)

	test := api.Group("/test")
	test.POST("/rooms", h.TestRooms)

	protected := api.Group("", AuthMiddleware(store, ns))

	protected.POST("/rooms", h.NewRoom)
	protected.POST("/rooms/:room_id/users/:user_fp", h.AddUsersToRoom)
	protected.DELETE("/rooms/:room_id/users/:user_fp", h.RemoveUserFromRoom)
	protected.POST("/rooms/:room_id/epoch-keys", h.SubmitEpochKeys)
	protected.GET("/rooms/:room_id/messages", h.GetRoomMessages)
	protected.GET("/rooms/:room_id/key-slots", h.GetRoomKeySlots)
	protected.GET("/users/me", h.UserInfo)
	protected.GET("/users", h.GetAllUsers)
	protected.GET("/users/me/rooms", h.GetUserRooms)
	protected.DELETE("/users/me/messages", h.PurgeUserMessages)
}
