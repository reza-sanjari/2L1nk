package handlers

import (
	"2L1nk/internal/hub"
	infradb "2L1nk/internal/infrastructure/db"
	"2L1nk/internal/models"
	"2L1nk/internal/service"
	"encoding/json"

	"go.uber.org/zap"
)

// buildRoomResponse constructs a roomResponse from DB record and optional live hub state.
func buildRoomResponse(dbRoom *infradb.RoomRecord, live *hub.UserRoomInfo) roomResponse {
	res := roomResponse{
		RoomID: dbRoom.ID,
		Name:   dbRoom.Name,
		Epoch:  dbRoom.CurrentEpoch,
		Online: live != nil,
	}
	if live != nil {
		res.Epoch = live.Epoch
		host := live.Host
		res.Host = &host
		res.Users = live.Users
	}
	return res
}

// membersToWithMode converts DB member key info to MemberWithMode using the stored mode.
func membersToWithMode(members []infradb.MemberKeyInfo) []service.MemberWithMode {
	out := make([]service.MemberWithMode, len(members))
	for i, m := range members {
		out[i] = service.MemberWithMode{FP: m.Fingerprint, Mode: models.UserMode(m.Mode)}
	}
	return out
}

// buildOnlineSet returns a map of fingerprint → true for all currently online users
// that are in the given member list.
func buildOnlineSet(members []service.MemberWithMode, h *hub.Hub) map[string]bool {
	online := make(map[string]bool, len(members))
	for _, m := range members {
		if h.IsUserOnline(m.FP) {
			online[m.FP] = true
		}
	}
	return online
}

// broadcastRoomUpdated sends a room_updated WS event to all online members.
// Uses DB as the authoritative member source for both persistent and ephemeral users.
func (h *Handler) broadcastRoomUpdated(roomID string, epoch int64, liveRoom *hub.UserRoomInfo) {
	if liveRoom == nil {
		return // room is offline; no one to notify
	}

	updPayload := hub.RoomUpdatedPayload{
		RoomID: liveRoom.RoomID,
		Name:   liveRoom.Name,
		Epoch:  epoch,
		Online: true,
		Host:   &liveRoom.Host,
	}

	members, err := h.services.Room.GetRoomMembersWithDetails(roomID)
	if err != nil {
		h.logg.Error("broadcastRoomUpdated: failed to fetch members", zap.String("roomID", roomID), zap.Error(err))
		return
	}
	userList := make([]hub.RoomMemberInfo, 0, len(members))
	for _, m := range members {
		userList = append(userList, hub.RoomMemberInfo{
			Fingerprint:      m.Fingerprint,
			Username:         m.Username,
			X25519PublicKey:  m.X25519PublicKey,
			Ed25519PublicKey: m.Ed25519PublicKey,
			Mode:             models.UserMode(m.Mode),
		})
	}
	updPayload.Users = userList

	payloadBytes, err := json.Marshal(updPayload)
	if err != nil {
		h.logg.Error("broadcastRoomUpdated: failed to marshal payload", zap.String("roomID", roomID), zap.Error(err))
		return
	}
	envelope, err := json.Marshal(map[string]any{
		"type":    models.RoomUpdated,
		"payload": json.RawMessage(payloadBytes),
	})
	if err != nil {
		h.logg.Error("broadcastRoomUpdated: failed to marshal envelope", zap.String("roomID", roomID), zap.Error(err))
		return
	}
	h.hub.BroadcastToRoom <- hub.BroadcastToRoomRequest{RoomID: roomID, Data: envelope}
}
