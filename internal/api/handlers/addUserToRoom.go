package handlers

import (
	"2L1nk/internal/hub"
	infradb "2L1nk/internal/infrastructure/db"
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

	// Resolve the new member's X25519 key and mode first — needed to decide
	// whether to promote an ephemeral room to DB (only on persistent member join).
	// DB is the primary source; fall back to hub for online ephemeral users.
	var memberX25519Key string
	var memberMode models.UserMode
	dbUser, err := h.services.Gate.GetUserByFingerprint(memberFP)
	if err != nil {
		h.logg.Error("add user to room: failed to look up user", zap.String("memberFP", memberFP), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	if dbUser != nil {
		memberX25519Key = dbUser.X25519PublicKey
		memberMode = models.UserModePersistent
	} else if onlineUser := h.hub.GetOnlineUser(memberFP); onlineUser != nil {
		memberX25519Key = onlineUser.X25519PublicKey
		memberMode = onlineUser.Mode
	} else {
		h.logg.Debug("add user to room: user not found", zap.String("memberFP", memberFP))
		return c.JSON(http.StatusNotFound, map[string]string{"error": "user not found"})
	}

	// Resolve the room record. DB is authoritative for persistent rooms.
	// If not in DB, fall back to the hub for ephemeral (temp-user-created) rooms.
	roomRecord, err := h.services.Room.GetRoomByID(roomID)
	if err != nil {
		h.logg.Error("add user to room: failed to fetch room", zap.String("roomID", roomID), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	if roomRecord == nil {
		// Room not in DB — check hub for a live ephemeral room.
		liveRoomInfo := h.hub.GetRoom(roomID)
		if liveRoomInfo == nil {
			h.logg.Debug("add user to room: room not found", zap.String("roomID", roomID))
			return c.JSON(http.StatusNotFound, map[string]string{"error": "room not found"})
		}
		if liveRoomInfo.Host.Fingerprint != caller.PublicKeyFingerprint {
			h.logg.Warn("add user to room: forbidden, caller is not the host", zap.String("roomID", roomID), zap.String("callerFP", caller.PublicKeyFingerprint), zap.String("hostFP", liveRoomInfo.Host.Fingerprint))
			return c.JSON(http.StatusForbidden, map[string]string{"error": "only the host can add members"})
		}

		if memberMode == models.UserModePersistent {
			// Promote the ephemeral room to DB so persistent data (members, messages,
			// key slots) can be stored from this point forward.
			if err := h.services.Room.PromoteEphemeralRoom(
				roomID,
				liveRoomInfo.Name,
				liveRoomInfo.Host.Fingerprint,
				liveRoomInfo.KeyCreatorFP,
				liveRoomInfo.Epoch,
				time.Now().Unix(),
			); err != nil {
				h.logg.Error("add user to room: failed to promote ephemeral room", zap.String("roomID", roomID), zap.Error(err))
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			}
			h.logg.Info("ephemeral room promoted to persistent", zap.String("roomID", roomID), zap.String("hostFP", liveRoomInfo.Host.Fingerprint))
			// Re-fetch so the rest of the handler uses the authoritative DB record.
			roomRecord, err = h.services.Room.GetRoomByID(roomID)
			if err != nil || roomRecord == nil {
				h.logg.Error("add user to room: failed to fetch promoted room", zap.String("roomID", roomID), zap.Error(err))
				return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
			}
		} else {
			// Adding an ephemeral user to an ephemeral room — synthesize a record from hub
			// state so the rest of the handler can proceed without DB knowledge.
			roomRecord = &infradb.RoomRecord{
				ID:           liveRoomInfo.RoomID,
				Name:         liveRoomInfo.Name,
				CurrentEpoch: liveRoomInfo.Epoch,
				HostFP:       liveRoomInfo.Host.Fingerprint,
				KeyCreatorFP: liveRoomInfo.KeyCreatorFP,
			}
		}
	} else {
		// Persistent room: validate host from DB.
		if roomRecord.HostFP != caller.PublicKeyFingerprint {
			h.logg.Warn("add user to room: forbidden, caller is not the host", zap.String("roomID", roomID), zap.String("callerFP", caller.PublicKeyFingerprint), zap.String("hostFP", roomRecord.HostFP))
			return c.JSON(http.StatusForbidden, map[string]string{"error": "only the host can add members"})
		}
	}

	// DB first: add member to room_members (silently skips ephemeral users and ephemeral rooms by design).
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

	liveRoom := h.hub.GetRoom(roomID)
	h.broadcastRoomUpdated(roomID, newEpoch, liveRoom)

	// Build member list from DB (authoritative, already updated) to avoid
	// the async hub race and the hub-only-tracks-online-members limitation.
	members, err := h.services.Room.GetRoomMembersWithDetails(roomID)
	if err != nil {
		h.logg.Error("add user to room: failed to fetch room members", zap.String("roomID", roomID), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	userList := make([]hub.RoomMemberInfo, 0, len(members))
	for _, m := range members {
		userList = append(userList, hub.RoomMemberInfo{
			Fingerprint:     m.Fingerprint,
			Username:        m.Username,
			X25519PublicKey: m.X25519PublicKey,
			Mode:            models.UserModePersistent,
		})
	}
	if liveRoom != nil {
		for _, u := range liveRoom.Users {
			if u.Mode == models.UserModeEphemeral {
				userList = append(userList, u)
			}
		}
	}

	resp := roomResponse{
		RoomID: roomRecord.ID,
		Name:   roomRecord.Name,
		Epoch:  newEpoch,
		Online: liveRoom != nil,
		Users:  userList,
	}
	if liveRoom != nil {
		host := liveRoom.Host
		resp.Host = &host
	}
	return c.JSON(http.StatusCreated, map[string]any{"room": resp})
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
