package handlers

import (
	"2L1nk/internal/hub"
	"net/http"

	"github.com/labstack/echo/v4"
)

func (h *Handler) NewRoom(c echo.Context) error {
	groupName := c.FormValue("groupName")

	respChan := make(chan string)

	h.Hub.RegisterRoom <- hub.CreateRoomRequest{
		GroupName:    groupName,
		ResponseChan: respChan,
	}

	roomID := <-respChan

	return c.JSON(http.StatusCreated, map[string]any{
		"room_id": roomID,
	})
}
