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

	for _, fp := range req.Users {
		h.hub.JoinRoom <- hub.RoomMembersChangeRequest{
			OwnerFP: user.PublicKeyFingerprint,
			RoomID:  roomID,
			UserFP:  fp,
		}
	}

	live := h.hub.GetRoom(roomID)
	if live == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "room not found"})
	}

	host := live.Host
	res := roomResponse{
		RoomID: live.RoomID,
		Name:   live.Name,
		Epoch:  live.Epoch,
		Online: true,
		Host:   &host,
		Users:  live.Users,
	}

	return c.JSON(http.StatusCreated, map[string]any{"room": res})
}
