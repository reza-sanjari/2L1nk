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

	// Verify caller is the host (DB check)
	roomRecord, err := h.services.Room.GetRoomByID(roomID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	if roomRecord == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "room not found"})
	}
	if roomRecord.KeyCreatorFP != user.PublicKeyFingerprint {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "only the host can add members"})
	}

	// Activate room in hub if it is currently offline
	if h.hub.GetRoom(roomID) == nil {
		members, err := h.services.Room.GetRoomMembers(roomID)
		if err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		}
		h.hub.RestoreRoom <- hub.RestoreRoomRequest{
			RoomID:    roomID,
			RoomName:  roomRecord.Name,
			HostFP:    roomRecord.KeyCreatorFP,
			Epoch:     roomRecord.CurrentEpoch,
			MemberFPs: members,
		}
	}

	// Add the new members
	for _, fp := range req.Users {
		h.hub.JoinRoom <- hub.RoomMembersChangeRequest{
			RoomID: roomID,
			UserFP: fp,
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
