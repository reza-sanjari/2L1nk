package handlers

import (
	"2L1nk/internal/session"
	"fmt"
	"net/http"

	"github.com/labstack/echo/v4"
)

func (h *Handler) RoomChange(c echo.Context) error {
	fmt.Println("roomChange called")
	roomId := c.Param("roomId")
	user := c.Get("user").(*session.User)
	return c.JSON(http.StatusCreated, map[string]any{
		"room_id": roomId,
		"user":    user.Username,
	})
}
