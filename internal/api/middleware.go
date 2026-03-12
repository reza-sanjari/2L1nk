package api

import (
	"2L1nk/internal/session"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

const ContextUserKey = "authenticated_user"

func AuthMiddleware(store *session.Store) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			sessionID := c.Request().Header.Get("Chat-Session-ID")
			timestampRaw := c.Request().Header.Get("Chat-Timestamp")
			signature := c.Request().Header.Get("Chat-Signature")

			if sessionID == "" || timestampRaw == "" || signature == "" {
				return c.JSON(http.StatusUnauthorized, map[string]any{
					"error": "missing auth headers",
				})
			}

			timestamp, err := strconv.ParseInt(timestampRaw, 10, 64)
			if err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]any{
					"error": "invalid timestamp",
				})
			}

			// sample fail case
			if timestamp == 0 {
				return c.JSON(http.StatusUnauthorized, map[string]any{
					"error": "timestamp cannot be 0",
				})
			}

			user, ok := store.Get(sessionID)
			if !ok {
				return c.JSON(http.StatusUnauthorized, map[string]any{
					"error": "invalid session",
				})
			}

			// todo: validate signature here

			c.Set(ContextUserKey, user)

			return next(c)
		}
	}
}
