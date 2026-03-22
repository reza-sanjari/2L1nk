package handlers

import (
	"2L1nk/internal/hub"
	"2L1nk/internal/session"
	"net/http"

	"github.com/labstack/echo/v4"
)

func (h *Handler) RemoveUserFromRoom(c echo.Context) error {
	roomID := c.Param("room_id")
	memberFP := c.Param("user_fp")
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
		return c.JSON(http.StatusForbidden, map[string]string{"error": "only the host can remove members"})
	}

	// Send remove command to hub (handles hub state + emits events for DB)
	h.hub.RemoveFromRoom <- hub.RemoveFromRoomRequest{
		RoomID:   roomID,
		MemberFP: memberFP,
	}

	live := h.hub.GetRoom(roomID)
	if live == nil {
		// Room was deleted or deactivated
		return c.JSON(http.StatusOK, map[string]any{"deleted": true})
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

	return c.JSON(http.StatusOK, map[string]any{"room": res})
}
