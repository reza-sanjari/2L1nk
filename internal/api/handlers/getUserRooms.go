package handlers

import (
	"2L1nk/internal/session"

	"github.com/labstack/echo/v4"
)

func (h *Handler) GetUserRooms(c echo.Context) error {
	user := c.Get("user").(*session.User)
	res := h.hub.GetUserRooms(user.PublicKeyFingerprint)

	return c.JSON(200, map[string]any{
		"rooms": res,
	})
}
