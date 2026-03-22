package handlers

import (
	"2L1nk/internal/hub"
	"2L1nk/internal/session"
	"net/http"

	"github.com/labstack/echo/v4"
)

type RemoveUserFromRoomRequest struct {
	UserFP string `json:"user_fp"`
}

func (h *Handler) RemoveUserFromRoom(c echo.Context) error {
	roomID := c.Param("room_id")

	var req RemoveUserFromRoomRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if req.UserFP == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "user_fp required"})
	}

	user := c.Get("user").(*session.User)

	h.hub.LeaveRoom <- hub.RoomMembersChangeRequest{
		OwnerFP: user.PublicKeyFingerprint,
		RoomID:  roomID,
		UserFP:  req.UserFP,
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
