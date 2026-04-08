package api

import (
	"2L1nk/internal/session"
	"2L1nk/internal/utils"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"
)

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

			now := time.Now().Unix()
			if timestamp < now-30 || timestamp > now+30 {
				return c.JSON(http.StatusUnauthorized, map[string]any{
					"error": "timestamp out of window",
				})
			}
			// TODO: add nonce store for full replay prevention

			user, ok := store.Get(sessionID)
			if !ok {
				return c.JSON(http.StatusUnauthorized, map[string]any{
					"error": "invalid session",
				})
			}

			var bodyBytes []byte
			if c.Request().Body != nil {
				bodyBytes, err = io.ReadAll(c.Request().Body)
				if err != nil {
					return c.JSON(http.StatusInternalServerError, map[string]any{
						"error": "failed to read body",
					})
				}
				c.Request().Body = io.NopCloser(bytes.NewReader(bodyBytes))
			}
			sum := sha256.Sum256(bodyBytes)
			bodyHash := hex.EncodeToString(sum[:])

			canonical := utils.HTTPCanonical(c.Request().Method, c.Request().URL.Path, timestampRaw, bodyHash)
			if err := utils.VerifySignature(user.PublicKey, canonical, signature); err != nil {
				return c.JSON(http.StatusUnauthorized, map[string]any{
					"error": "invalid signature",
				})
			}

			c.Set("user", user)

			return next(c)
		}
	}
}
