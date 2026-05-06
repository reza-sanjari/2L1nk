package hub

import (
	"2L1nk/internal/models"
	"encoding/base64"
)

type UserStatus struct {
	Username    string `json:"username"`
	Fingerprint string `json:"fingerprint"`
	Online      bool   `json:"online"`
}

type RoomMemberInfo struct {
	Username         string          `json:"username"`
	Fingerprint      string          `json:"fingerprint"`
	Mode             models.UserMode `json:"mode"`
	X25519PublicKey  string          `json:"x25519_public_key"`
	Ed25519PublicKey string          `json:"ed25519_public_key"`
}

type UserRoomInfo struct {
	RoomID       string           `json:"room_id"`
	Name         string           `json:"name"`
	Host         RoomMemberInfo   `json:"host"`
	Users        []RoomMemberInfo `json:"users"`
	Epoch        int64            `json:"epoch"`
	KeyCreatorFP string           `json:"key_creator_fp"`
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

// roomHostInfo builds a RoomMemberInfo for the host, safe when Host is nil.
func roomHostInfo(room *Room) RoomMemberInfo {
	info := RoomMemberInfo{
		Fingerprint: room.HostFP,
		Username:    room.HostName,
	}
	if room.Host != nil {
		info.Mode = room.Host.Mode
		info.X25519PublicKey = room.Host.X25519PublicKey
		info.Ed25519PublicKey = base64.StdEncoding.EncodeToString(room.Host.Ed25519PublicKey)
	} else if pk, ok := room.MemberEd25519Keys[room.HostFP]; ok {
		info.Ed25519PublicKey = pk
	}
	return info
}

// keyCreatorFP returns the current key creator fingerprint.
func keyCreatorFP(room *Room) string {
	if room.PendingRotation != nil {
		return room.PendingRotation.KeyCreatorFP
	}
	return room.KeyCreatorFP
}

// IsUserOnline reports whether the given fingerprint has an active WS connection.
func (h *Hub) IsUserOnline(fp string) bool {
	_, ok := h.Users[fp]
	return ok
}

// GetOnlineUser returns the User for a given fingerprint if they are currently connected, or nil.
func (h *Hub) GetOnlineUser(fp string) *User {
	return h.Users[fp]
}

func (h *Hub) GetRoom(roomID string) *UserRoomInfo {
	room, ok := h.Rooms[roomID]
	if !ok {
		return nil
	}

	var users []RoomMemberInfo
	for _, user := range room.Users {
		users = append(users, RoomMemberInfo{
			Username:         user.Username,
			Fingerprint:      user.Fingerprint,
			Mode:             user.Mode,
			X25519PublicKey:  user.X25519PublicKey,
			Ed25519PublicKey: base64.StdEncoding.EncodeToString(user.Ed25519PublicKey),
		})
	}

	return &UserRoomInfo{
		RoomID:       room.RoomID,
		Name:         room.Name,
		Host:         roomHostInfo(room),
		Users:        users,
		Epoch:        room.Epoch,
		KeyCreatorFP: keyCreatorFP(room),
	}
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
				Username:         user.Username,
				Fingerprint:      user.Fingerprint,
				Mode:             user.Mode,
				X25519PublicKey:  user.X25519PublicKey,
				Ed25519PublicKey: base64.StdEncoding.EncodeToString(user.Ed25519PublicKey),
			})
		}

		rooms = append(rooms, UserRoomInfo{
			RoomID:       room.RoomID,
			Name:         room.Name,
			Host:         roomHostInfo(room),
			Users:        users,
			Epoch:        room.Epoch,
			KeyCreatorFP: keyCreatorFP(room),
		})
	}

	return rooms
}

// GetPendingRotation returns the pending rotation for a room, or nil if none.
func (h *Hub) GetPendingRotation(roomID string) *PendingRotation {
	if room, ok := h.Rooms[roomID]; ok {
		return room.PendingRotation
	}
	return nil
}

// GetMemberRooms returns the IDs of all hub rooms where fp appears in MemberModes
// (i.e. is a known member, regardless of whether they are currently online).
// Used on WS reconnect to re-slot users into their active rooms.
func (h *Hub) GetMemberRooms(fp string) []string {
	var ids []string
	for _, room := range h.Rooms {
		if _, ok := room.MemberModes[fp]; ok {
			ids = append(ids, room.RoomID)
		}
	}
	return ids
}
