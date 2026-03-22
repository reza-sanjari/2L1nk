package handlers

import (
	"2L1nk/internal/models"
	"2L1nk/internal/session"
	"net/http"

	"github.com/labstack/echo/v4"
)

func (h *Handler) UserInfo(c echo.Context) error {
	user := c.Get("user").(*session.User)

	if user.Mode == models.UserModePersistent {
		record, err := h.services.Gate.GetUserByFingerprint(user.PublicKeyFingerprint)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{
				"error": "internal server error",
			})
		}
		if record != nil {
			return c.JSON(http.StatusOK, map[string]any{
				"username":          record.Username,
				"publicFingerPrint": record.Fingerprint,
				"mode":              user.Mode,
				"createdAt":         record.CreatedAt,
			})
		}
	}

	return c.JSON(http.StatusOK, map[string]any{
		"username":          user.Username,
		"publicFingerPrint": user.PublicKeyFingerprint,
		"mode":              user.Mode,
	})
}
