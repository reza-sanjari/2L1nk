package hub

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
