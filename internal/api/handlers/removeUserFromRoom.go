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
	caller := c.Get("user").(*session.User)

	// Validate caller is the host.
	roomRecord, err := h.services.Room.GetRoomByID(roomID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	if roomRecord == nil {
		return c.JSON(http.StatusNotFound, map[string]string{"error": "room not found"})
	}
	if roomRecord.HostFP != caller.PublicKeyFingerprint {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "only the host can remove members"})
	}

	// Check if this is the last member.
	members, err := h.services.Room.GetRoomMembers(roomID)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	isLast := len(members) <= 1

	if isLast {
		// Case A: last member — delete everything from DB.
		if err := h.services.Room.DeleteKeySlotsByRoom(roomID); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		}
		if err := h.services.Message.DeleteByRoom(roomID); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		}
		if err := h.services.Room.DeleteRoom(roomID); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		}
		// Hub sync: remove room from hub if active.
		h.hub.RemoveFromRoom <- hub.RemoveFromRoomRequest{
			RoomID:  roomID,
			Deleted: true,
		}
		return c.JSON(http.StatusOK, map[string]any{"deleted": true})
	}

	// Case B/C: remove member from DB.
	if err := h.services.Room.RemoveMember(roomID, memberFP); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	// Determine new host if the removed member was the host.
	newHostFP := ""
	if memberFP == roomRecord.HostFP {
		newHostFP = selectKeyCreatorAfterChange(roomID, "", memberFP, h.services.Room, h.hub)
		if newHostFP == "" {
			// Fallback: lex lowest remaining member from DB.
			remaining, _ := h.services.Room.GetRoomMembers(roomID)
			if len(remaining) > 0 {
				newHostFP = remaining[0]
				for _, fp := range remaining {
					if fp < newHostFP {
						newHostFP = fp
					}
				}
			}
		}
		if err := h.services.Room.UpdateHostFP(roomID, newHostFP); err != nil {
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		}
	}

	// Determine new key creator.
	newKeyCreatorFP := selectKeyCreatorAfterChange(roomID, roomRecord.KeyCreatorFP, memberFP, h.services.Room, h.hub)
	if newKeyCreatorFP == "" {
		newKeyCreatorFP = roomRecord.KeyCreatorFP
	}

	newEpoch := roomRecord.CurrentEpoch + 1
	if err := h.services.Room.UpdateEpochAndKeyCreator(roomID, newEpoch, newKeyCreatorFP); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	// Hub sync: remove member and update state.
	h.hub.RemoveFromRoom <- hub.RemoveFromRoomRequest{
		RoomID:          roomID,
		MemberFP:        memberFP,
		Deleted:         false,
		NewEpoch:        newEpoch,
		NewKeyCreatorFP: newKeyCreatorFP,
		NewHostFP:       newHostFP,
	}

	updatedRoom, err := h.services.Room.GetRoomByID(roomID)
	if err != nil || updatedRoom == nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	return c.JSON(http.StatusOK, map[string]any{"room": buildRoomResponse(updatedRoom, h.hub.GetRoom(roomID))})
}
