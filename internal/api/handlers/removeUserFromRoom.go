package handlers

import (
	"2L1nk/internal/hub"
	"2L1nk/internal/session"
	"net/http"
	"slices"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

func (h *Handler) RemoveUserFromRoom(c echo.Context) error {
	roomID := c.Param("room_id")
	memberFP := c.Param("user_fp")
	caller := c.Get("user").(*session.User)

	h.logg.Debug("remove user from room request", zap.String("roomID", roomID), zap.String("memberFP", memberFP), zap.String("callerFP", caller.PublicKeyFingerprint))

	// Validate caller is the host.
	roomRecord, err := h.services.Room.GetRoomByID(roomID)
	if err != nil {
		h.logg.Error("remove user from room: failed to fetch room", zap.String("roomID", roomID), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	if roomRecord == nil {
		h.logg.Debug("remove user from room: room not found", zap.String("roomID", roomID))
		return c.JSON(http.StatusNotFound, map[string]string{"error": "room not found"})
	}
	if roomRecord.HostFP != caller.PublicKeyFingerprint {
		h.logg.Warn("remove user from room: forbidden, caller is not the host", zap.String("roomID", roomID), zap.String("callerFP", caller.PublicKeyFingerprint), zap.String("hostFP", roomRecord.HostFP))
		return c.JSON(http.StatusForbidden, map[string]string{"error": "only the host can remove members"})
	}

	// Check if this is the last member.
	members, err := h.services.Room.GetRoomMembers(roomID)
	if err != nil {
		h.logg.Error("remove user from room: failed to fetch members", zap.String("roomID", roomID), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	if !slices.Contains(members, memberFP) {
		h.logg.Debug("remove user from room: user is not a member", zap.String("roomID", roomID), zap.String("memberFP", memberFP))
		return c.JSON(http.StatusNotFound, map[string]string{"error": "user is not a member of this room"})
	}

	isLast := len(members) <= 1
	h.logg.Debug("remove user from room: member count checked", zap.String("roomID", roomID), zap.Int("count", len(members)), zap.Bool("isLast", isLast))

	if isLast {
		// Case A: last member — delete everything from DB.
		if err := h.services.Room.DeleteKeySlotsByRoom(roomID); err != nil {
			h.logg.Error("remove user from room: failed to delete key slots", zap.String("roomID", roomID), zap.Error(err))
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		}
		if err := h.services.Message.DeleteByRoom(roomID); err != nil {
			h.logg.Error("remove user from room: failed to delete messages", zap.String("roomID", roomID), zap.Error(err))
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		}
		if err := h.services.Room.DeleteRoom(roomID); err != nil {
			h.logg.Error("remove user from room: failed to delete room", zap.String("roomID", roomID), zap.Error(err))
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		}
		// Hub sync: remove room from hub if active.
		h.hub.RemoveFromRoom <- hub.RemoveFromRoomRequest{
			RoomID:  roomID,
			Deleted: true,
		}
		h.logg.Info("room deleted (last member removed)", zap.String("roomID", roomID), zap.String("callerFP", caller.PublicKeyFingerprint))
		return c.JSON(http.StatusOK, map[string]any{"deleted": true})
	}

	// Case B/C: remove member from DB.
	if err := h.services.Room.RemoveMember(roomID, memberFP); err != nil {
		h.logg.Error("remove user from room: failed to remove member", zap.String("roomID", roomID), zap.String("memberFP", memberFP), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	h.logg.Debug("remove user from room: member removed from DB", zap.String("roomID", roomID), zap.String("memberFP", memberFP))

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
			h.logg.Error("remove user from room: failed to update host", zap.String("roomID", roomID), zap.Error(err))
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		}
		newHostUsername := newHostFP // fallback to FP if new host is offline
		if u := h.hub.GetOnlineUser(newHostFP); u != nil {
			newHostUsername = u.Username
		} else if dbUser, err := h.services.Gate.GetUserByFingerprint(newHostFP); err == nil && dbUser != nil {
			newHostUsername = dbUser.Username
		}
		h.logg.Info("group owner changed",
			zap.String("roomID", roomID),
			zap.String("oldHostFP", memberFP),
			zap.String("newHostFP", newHostFP),
			zap.String("newHostUsername", newHostUsername),
		)
	}

	// Determine new key creator.
	newKeyCreatorFP := selectKeyCreatorAfterChange(roomID, roomRecord.KeyCreatorFP, memberFP, h.services.Room, h.hub)
	if newKeyCreatorFP == "" {
		newKeyCreatorFP = roomRecord.KeyCreatorFP
	}

	newEpoch := roomRecord.CurrentEpoch + 1
	if err := h.services.Room.UpdateEpochAndKeyCreator(roomID, newEpoch, newKeyCreatorFP); err != nil {
		h.logg.Error("remove user from room: failed to update epoch and key creator", zap.String("roomID", roomID), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	h.logg.Debug("remove user from room: epoch updated", zap.String("roomID", roomID), zap.Int64("newEpoch", newEpoch), zap.String("keyCreatorFP", newKeyCreatorFP))

	// Hub sync: remove member and update state.
	h.hub.RemoveFromRoom <- hub.RemoveFromRoomRequest{
		RoomID:          roomID,
		MemberFP:        memberFP,
		Deleted:         false,
		NewEpoch:        newEpoch,
		NewKeyCreatorFP: newKeyCreatorFP,
		NewHostFP:       newHostFP,
	}

	h.logg.Info("user removed from room", zap.String("roomID", roomID), zap.String("memberFP", memberFP), zap.Int64("newEpoch", newEpoch))

	h.broadcastRoomUpdated(roomID, newEpoch, h.hub.GetRoom(roomID))

	updatedRoom, err := h.services.Room.GetRoomByID(roomID)
	if err != nil || updatedRoom == nil {
		h.logg.Error("remove user from room: failed to fetch updated room", zap.String("roomID", roomID), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	return c.JSON(http.StatusOK, map[string]any{"room": buildRoomResponse(updatedRoom, h.hub.GetRoom(roomID))})
}
