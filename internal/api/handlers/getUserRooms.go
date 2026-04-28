package handlers

import (
	"2L1nk/internal/hub"
	"2L1nk/internal/models"
	"2L1nk/internal/session"
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

type roomResponse struct {
	RoomID string               `json:"room_id"`
	Name   string               `json:"name"`
	Epoch  int64                `json:"epoch"`
	Online bool                 `json:"online"`
	Host   *hub.RoomMemberInfo  `json:"host,omitempty"`
	Users  []hub.RoomMemberInfo `json:"users,omitempty"`
}

func (h *Handler) GetUserRooms(c echo.Context) error {
	user := c.Get("user").(*session.User)

	h.logg.Debug("get user rooms request", zap.String("userFP", user.PublicKeyFingerprint), zap.Int("mode", int(user.Mode)))

	dbRooms, err := h.services.Room.GetUserRooms(user.PublicKeyFingerprint)
	if err != nil {
		h.logg.Error("get user rooms: failed to fetch rooms from DB", zap.String("userFP", user.PublicKeyFingerprint), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	// Index live hub rooms by ID for O(1) merge lookup.
	hubRooms := h.hub.GetUserRooms(user.PublicKeyFingerprint)
	hubMap := make(map[string]hub.UserRoomInfo, len(hubRooms))
	for _, r := range hubRooms {
		hubMap[r.RoomID] = r
	}

	rooms := make([]roomResponse, 0, len(dbRooms))
	for _, dbRoom := range dbRooms {
		members, err := h.services.Room.GetRoomMembersWithDetails(dbRoom.ID)
		if err != nil {
			h.logg.Error("get user rooms: failed to fetch members", zap.String("roomID", dbRoom.ID), zap.Error(err))
			return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
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

		r := roomResponse{
			RoomID: dbRoom.ID,
			Name:   dbRoom.Name,
			Epoch:  dbRoom.CurrentEpoch,
			Online: false,
			Users:  userList,
		}
		if live, ok := hubMap[dbRoom.ID]; ok {
			r.Epoch = live.Epoch
			r.Online = true
			host := live.Host
			r.Host = &host
		}
		rooms = append(rooms, r)
	}

	h.logg.Debug("get user rooms: returning rooms", zap.String("userFP", user.PublicKeyFingerprint), zap.Int("total", len(rooms)))
	return c.JSON(http.StatusOK, map[string]any{"rooms": rooms})
}
