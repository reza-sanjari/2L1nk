package handlers

import (
	"2L1nk/internal/hub"
	"2L1nk/internal/session"
	"net/http"

	"github.com/labstack/echo/v4"
)

type CreateRoomRequest struct {
	GroupName string `json:"groupName"`
}

func (h *Handler) NewRoom(c echo.Context) error {
	var req CreateRoomRequest

	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]string{"error": "invalid body"})
	}

	groupName := req.GroupName
	
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
