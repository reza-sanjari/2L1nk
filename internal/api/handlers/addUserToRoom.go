package handlers

import (
	"2L1nk/internal/hub"
	"2L1nk/internal/models"
	"2L1nk/internal/service"
	"2L1nk/internal/session"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

func (h *Handler) AddUsersToRoom(c echo.Context) error {
	roomID := c.Param("room_id")
	memberFP := c.Param("user_fp")
	caller := c.Get("user").(*session.User)

	h.logg.Debug("add user to room request", zap.String("roomID", roomID), zap.String("memberFP", memberFP), zap.String("callerFP", caller.PublicKeyFingerprint))

	// Validate caller is the host.
	roomRecord, err := h.services.Room.GetRoomByID(roomID)
	if err != nil {
		h.logg.Error("add user to room: failed to fetch room", zap.String("roomID", roomID), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	if roomRecord == nil {
		h.logg.Debug("add user to room: room not found", zap.String("roomID", roomID))
		return c.JSON(http.StatusNotFound, map[string]string{"error": "room not found"})
	}
	if roomRecord.HostFP != caller.PublicKeyFingerprint {
		h.logg.Warn("add user to room: forbidden, caller is not the host", zap.String("roomID", roomID), zap.String("callerFP", caller.PublicKeyFingerprint), zap.String("hostFP", roomRecord.HostFP))
		return c.JSON(http.StatusForbidden, map[string]string{"error": "only the host can add members"})
	}

	// DB first: add member to room_members.
	if err := h.services.Room.AddMemberDirect(roomID, memberFP, time.Now().Unix()); err != nil {
		h.logg.Error("add user to room: failed to add member to DB", zap.String("roomID", roomID), zap.String("memberFP", memberFP), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	h.logg.Debug("add user to room: member added to DB", zap.String("roomID", roomID), zap.String("memberFP", memberFP))

	// Determine the new key creator.
	newKeyCreatorFP := selectKeyCreatorAfterChange(roomID, roomRecord.KeyCreatorFP, "", h.services.Room, h.hub)
	if newKeyCreatorFP == "" {
		newKeyCreatorFP = roomRecord.KeyCreatorFP
	}

	newEpoch := roomRecord.CurrentEpoch + 1
	if err := h.services.Room.UpdateEpochAndKeyCreator(roomID, newEpoch, newKeyCreatorFP); err != nil {
		h.logg.Error("add user to room: failed to update epoch and key creator", zap.String("roomID", roomID), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	h.logg.Debug("add user to room: epoch updated", zap.String("roomID", roomID), zap.Int64("newEpoch", newEpoch), zap.String("keyCreatorFP", newKeyCreatorFP))

	// Find the new member's X25519 key and mode for hub sync.
	var memberX25519Key string
	var memberMode models.UserMode
	if liveRoom := h.hub.GetRoom(roomID); liveRoom != nil {
		for _, u := range liveRoom.Users {
			if u.Fingerprint == memberFP {
				memberX25519Key = u.X25519PublicKey
				memberMode = u.Mode
				break
			}
		}
	}

	// Hub sync: sends to JoinRoom channel; hub updates state and broadcasts rotation WS.
	h.hub.JoinRoom <- hub.RoomMembersChangeRequest{
		RoomID:          roomID,
		UserFP:          memberFP,
		UserMode:        memberMode,
		UserX25519Key:   memberX25519Key,
		NewEpoch:        newEpoch,
		NewKeyCreatorFP: newKeyCreatorFP,
	}

	h.logg.Info("user added to room", zap.String("roomID", roomID), zap.String("memberFP", memberFP), zap.Int64("newEpoch", newEpoch))

	updatedRoom, err := h.services.Room.GetRoomByID(roomID)
	if err != nil || updatedRoom == nil {
		h.logg.Error("add user to room: failed to fetch updated room", zap.String("roomID", roomID), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	return c.JSON(http.StatusCreated, map[string]any{"room": buildRoomResponse(updatedRoom, h.hub.GetRoom(roomID))})
}

// selectKeyCreatorAfterChange picks the key creator for the updated room.
// Keeps the current creator if they are online; otherwise runs lex selection.
// excludeFP is the member being removed (or "" for an add).
func selectKeyCreatorAfterChange(
	roomID, currentCreatorFP, excludeFP string,
	roomSvc *service.RoomService,
	h *hub.Hub,
) string {
	// Keep existing creator if they are still a member and online.
	if excludeFP != currentCreatorFP && h.IsUserOnline(currentCreatorFP) {
		return currentCreatorFP
	}
	// Need to reassign — query remaining persistent members from DB.
	members, err := roomSvc.GetMembersWithPublicKeys(roomID)
	if err != nil {
		return ""
	}
	memberList := persistentMembersToWithMode(members)
	// Online ephemeral members in the hub room are also eligible.
	if liveRoom := h.GetRoom(roomID); liveRoom != nil {
		for _, u := range liveRoom.Users {
			memberList = appendIfMissing(memberList, u.Fingerprint, u.Mode)
		}
	}
	onlineSet := buildOnlineSet(memberList, h)
	return service.SelectNextByLex(memberList, onlineSet, excludeFP)
}
