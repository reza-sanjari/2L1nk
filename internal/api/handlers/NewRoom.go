package handlers

import (
	"2L1nk/internal/hub"
	"2L1nk/internal/session"
	"net/http"

	"github.com/labstack/echo/v4"
)

func (h *Handler) NewRoom(c echo.Context) error {
	groupName := c.FormValue("groupName")

	respChan := make(chan string)

	h.hub.RegisterRoom <- hub.CreateRoomRequest{
		Host:         c.Get("user").(*session.User),
		GroupName:    groupName,
		ResponseChan: respChan,
	}

	roomID := <-respChan
	if roomID == "" {
		return c.JSON(http.StatusInternalServerError, "Room creation failed")
	}
	return c.JSON(http.StatusCreated, map[string]any{
		"room_id": roomID,
	})
}
