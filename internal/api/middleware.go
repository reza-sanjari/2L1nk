package api

import (
	"2L1nk/internal/session"
	"2L1nk/internal/utils"
	"bytes"
	"io"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

// SessionMiddleware resolves the user from the Chat-Session-ID header and
// stores it on the echo context under "user". It is the only middleware
// responsible for identity lookup; signature verification is a separate step.
func SessionMiddleware(store *session.Store) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			sessionID := c.Request().Header.Get("Chat-Session-ID")
			if sessionID == "" {
				return c.JSON(http.StatusUnauthorized, map[string]any{
					"error": "missing auth headers",
				})
			}

			user, ok := store.Get(sessionID)
			if !ok {
				return c.JSON(http.StatusUnauthorized, map[string]any{
					"error": "invalid session",
				})
			}

			c.Set("user", user)
			return next(c)
		}
	}
}

// SignatureMiddleware verifies the Ed25519 signature on the request against
// the public key of the user set by SessionMiddleware. It also enforces the
// timestamp window and rejects replayed signatures via the nonce store.
// Must be applied after SessionMiddleware.
func SignatureMiddleware(ns *utils.NonceStore) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			timestampRaw := c.Request().Header.Get("Chat-Timestamp")
			signature := c.Request().Header.Get("Chat-Signature")
			nonce := c.Request().Header.Get("Chat-Nonce")

			if timestampRaw == "" || signature == "" || nonce == "" {
				return c.JSON(http.StatusUnauthorized, map[string]any{
					"error": "missing auth headers",
				})
			}

			user, ok := c.Get("user").(*session.User)
			if !ok || user == nil {
				return c.JSON(http.StatusUnauthorized, map[string]any{
					"error": "invalid session",
				})
			}

			var bodyBytes []byte
			if c.Request().Body != nil {
				var err error
				bodyBytes, err = io.ReadAll(c.Request().Body)
				if err != nil {
					return c.JSON(http.StatusInternalServerError, map[string]any{
						"error": "failed to read body",
					})
				}
				c.Request().Body = io.NopCloser(bytes.NewReader(bodyBytes))
			}
			bodyHash := utils.HashBodyHex(bodyBytes)

			canonical := utils.HTTPCanonical(c.Request().Method, c.Request().URL.Path, timestampRaw, bodyHash, nonce)
			if err := utils.VerifySignedRequest(user.PublicKey, canonical, timestampRaw, signature, signature, 30*time.Second, ns); err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]any{
					"error": err.Error(),
				})
			}

			return next(c)
		}
	}
}

// AuthMiddleware is a composed shim of SessionMiddleware + SignatureMiddleware
// kept for callers that want both in one line.
func AuthMiddleware(store *session.Store, ns *utils.NonceStore) echo.MiddlewareFunc {
	sessionMW := SessionMiddleware(store)
	sigMW := SignatureMiddleware(ns)
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return sessionMW(sigMW(next))
	}
}
