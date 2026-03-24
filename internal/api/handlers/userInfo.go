package handlers

import (
	"2L1nk/internal/models"
	"2L1nk/internal/session"
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

func (h *Handler) UserInfo(c echo.Context) error {
	user := c.Get("user").(*session.User)

	h.logg.Debug("user info request", zap.String("userFP", user.PublicKeyFingerprint), zap.Int("mode", int(user.Mode)))

	if user.Mode == models.UserModePersistent {
		record, err := h.services.Gate.GetUserByFingerprint(user.PublicKeyFingerprint)
		if err != nil {
			h.logg.Error("user info: failed to fetch user from DB", zap.String("userFP", user.PublicKeyFingerprint), zap.Error(err))
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
		}
		if record != nil {
			h.logg.Debug("user info: returning persistent user record", zap.String("userFP", user.PublicKeyFingerprint))
			return c.JSON(http.StatusOK, map[string]any{
				"username":          record.Username,
				"publicFingerPrint": record.Fingerprint,
				"mode":              user.Mode,
				"createdAt":         record.CreatedAt,
			})
		}
	}

	h.logg.Debug("user info: returning session user", zap.String("userFP", user.PublicKeyFingerprint))
	return c.JSON(http.StatusOK, map[string]any{
		"username":          user.Username,
		"publicFingerPrint": user.PublicKeyFingerprint,
		"mode":              user.Mode,
	})
}
