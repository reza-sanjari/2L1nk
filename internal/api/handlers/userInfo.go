package handlers

import (
	"2L1nk/internal/session"
	"net/http"

	"github.com/labstack/echo/v4"
)

func (h *Handler) UserInfo(c echo.Context) error {
	user := c.Get("user").(*session.User)
	return c.JSON(http.StatusCreated, map[string]any{
		"username":          user.Username,
		"publicFingerPrint": user.PublicKeyFingerprint,
		"mode":              user.Mode,
	})
}
