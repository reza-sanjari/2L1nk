package handlers

import (
	"2L1nk/internal/hub"
	"2L1nk/internal/models"
	"2L1nk/internal/session"
	"net/http"

	"github.com/labstack/echo/v4"
)

func (h *Handler) GetUserRooms(c echo.Context) error {
	user := c.Get("user").(*session.User)

	if user.Mode != models.UserModePersistent {
		res := h.hub.GetUserRooms(user.PublicKeyFingerprint)
		return c.JSON(http.StatusOK, map[string]any{"rooms": res})
	}

	dbRooms, err := h.services.Room.GetUserRooms(user.PublicKeyFingerprint)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	// Index live hub rooms by ID for O(1) merge lookup.
	hubRooms := h.hub.GetUserRooms(user.PublicKeyFingerprint)
	hubMap := make(map[string]hub.UserRoomInfo, len(hubRooms))
	for _, r := range hubRooms {
		hubMap[r.RoomID] = r
	}

	type roomResponse struct {
		RoomID string               `json:"room_id"`
		Name   string               `json:"name"`
		Epoch  int64                `json:"epoch"`
		Online bool                 `json:"online"`
		Host   *hub.RoomMemberInfo  `json:"host,omitempty"`
		Users  []hub.RoomMemberInfo `json:"users,omitempty"`
	}

	rooms := make([]roomResponse, 0, len(dbRooms))
	for _, dbRoom := range dbRooms {
		r := roomResponse{
			RoomID: dbRoom.ID,
			Name:   dbRoom.Name,
			Epoch:  dbRoom.CurrentEpoch,
			Online: false,
		}
		if live, ok := hubMap[dbRoom.ID]; ok {
			r.Epoch = live.Epoch
			r.Online = true
			host := live.Host
			r.Host = &host
			r.Users = live.Users
		}
		rooms = append(rooms, r)
	}

	return c.JSON(http.StatusOK, map[string]any{"rooms": rooms})
}
