package api

import (
	"2L1nk/internal/session"
	"net/http"

	"github.com/labstack/echo/v4"
)

func SessionAuthMiddleware(store *session.Store) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			sessionID := c.Request().Header.Get("X-Session-ID")
			if sessionID == "" {
				return echo.NewHTTPError(http.StatusUnauthorized, "missing session ID")
			}

			user, ok := store.Get(sessionID)
			if !ok {
				return echo.NewHTTPError(http.StatusUnauthorized, "invalid session ID")
			}

			// Inject user into context for handlers to use
			c.Set("user", user)
			return next(c)
		}
	}
}
