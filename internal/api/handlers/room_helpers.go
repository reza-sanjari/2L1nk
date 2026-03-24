package handlers

import (
	"2L1nk/internal/hub"
	infradb "2L1nk/internal/infrastructure/db"
	"2L1nk/internal/models"
	"2L1nk/internal/service"
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

// persistentMembersToWithMode converts DB member key info (all persistent) to MemberWithMode.
func persistentMembersToWithMode(members []infradb.MemberKeyInfo) []service.MemberWithMode {
	out := make([]service.MemberWithMode, len(members))
	for i, m := range members {
		out[i] = service.MemberWithMode{FP: m.Fingerprint, Mode: models.UserModePersistent}
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

// appendIfMissing adds a MemberWithMode entry if the FP is not already in the list.
func appendIfMissing(members []service.MemberWithMode, fp string, mode models.UserMode) []service.MemberWithMode {
	for _, m := range members {
		if m.FP == fp {
			return members
		}
	}
	return append(members, service.MemberWithMode{FP: fp, Mode: mode})
}
