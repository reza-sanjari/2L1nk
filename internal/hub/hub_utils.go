package hub

import "2L1nk/internal/models"

type UserStatus struct {
	Username    string `json:"username"`
	Fingerprint string `json:"fingerprint"`
	Online      bool   `json:"online"`
}

type RoomMemberInfo struct {
	Username    string          `json:"username"`
	Fingerprint string          `json:"fingerprint"`
	Mode        models.UserMode `json:"mode"`
}

type UserRoomInfo struct {
	RoomID string           `json:"room_id"`
	Name   string           `json:"name"`
	Host   RoomMemberInfo   `json:"host"`
	Users  []RoomMemberInfo `json:"users"`
	Epoch  int64            `json:"epoch"`
}

func (h *Hub) getUser(fingerprint string) *User {
	return h.Users[fingerprint]
}

func (h *Hub) isUserInRoom(user *User, room *Room) bool {
	_, ok := room.Users[user.Fingerprint]
	return ok
}

func (h *Hub) getRoom(roomId string) *Room {
	return h.Rooms[roomId]
}

func (h *Hub) sendMessageToRoom(targetRoom *Room, data []byte) {
	for _, user := range targetRoom.Users {
		user.OutGoingMessages <- data
	}
}

func (h *Hub) GetUsers() []UserStatus {
	var users []UserStatus

	for _, user := range h.Users {
		users = append(users, UserStatus{
			Username:    user.Username,
			Fingerprint: user.Fingerprint,
			Online:      true,
		})
	}

	return users
}

func (h *Hub) GetRoom(roomID string) *UserRoomInfo {
	room, ok := h.Rooms[roomID]
	if !ok {
		return nil
	}

	var users []RoomMemberInfo
	for _, user := range room.Users {
		users = append(users, RoomMemberInfo{
			Username:    user.Username,
			Fingerprint: user.Fingerprint,
			Mode:        user.Mode,
		})
	}

	info := &UserRoomInfo{
		RoomID: room.RoomID,
		Name:   room.Name,
		Host: RoomMemberInfo{
			Username:    room.Host.Username,
			Fingerprint: room.Host.Fingerprint,
			Mode:        room.Host.Mode,
		},
		Users: users,
		Epoch: room.Epoch,
	}
	return info
}

func (h *Hub) GetUserRooms(userFingerprint string) []UserRoomInfo {
	var rooms []UserRoomInfo

	for _, room := range h.Rooms {
		if _, ok := room.Users[userFingerprint]; !ok {
			continue
		}

		var users []RoomMemberInfo
		for _, user := range room.Users {
			users = append(users, RoomMemberInfo{
				Username:    user.Username,
				Fingerprint: user.Fingerprint,
				Mode:        user.Mode,
			})
		}

		rooms = append(rooms, UserRoomInfo{
			RoomID: room.RoomID,
			Name:   room.Name,
			Host: RoomMemberInfo{
				Username:    room.Host.Username,
				Fingerprint: room.Host.Fingerprint,
				Mode:        room.Host.Mode,
			},
			Users: users,
			Epoch: room.Epoch,
		})
	}

	return rooms
}
