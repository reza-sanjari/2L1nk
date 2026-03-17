package handlers

import (
	"2L1nk/internal/hub"
	"2L1nk/internal/session"
	"net/http"

	"github.com/labstack/echo/v4"
)

type AddUsersToRoomRequest struct {
	Users []string `json:"users"`
}

func (h *Handler) AddUsersToRoom(c echo.Context) error {
	roomID := c.Param("room_id")
	var req AddUsersToRoomRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "invalid request body")
	}

	user := c.Get("user").(*session.User)

	for u := range req.Users {
		h.hub.JoinRoom <- hub.RoomMembersChangeRequest{
			OwnerFP: user.PublicKeyFingerprint,
			RoomID:  roomID,
			UserFP:  req.Users[u],
		}
	}

	return c.JSON(http.StatusCreated, map[string]any{
		"room":  roomID,
		"users": req.Users,
		"user":  user.Username,
	})
}
