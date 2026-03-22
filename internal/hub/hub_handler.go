package hub

import (
	"2L1nk/internal/models"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

func (h *Hub) handleInboundMessage(msg WSMessageEnvelope) {
	var payload MessagePayload
	err := json.Unmarshal(msg.Payload, &payload)
	h.logg.Debug("message received", zap.String("user", msg.Sender.Username))
	if err != nil {
		h.logg.Error("Failed to unmarshal payload", zap.String("payload", string(msg.Payload)), zap.Error(err))
		return
	}

	targetRoom := h.getRoom(payload.RoomID)
	if targetRoom == nil {
		// Room is offline — emit event so the event consumer can load it from DB and deliver
		h.logg.Debug("target room not in hub, emitting room_offline", zap.String("roomId", payload.RoomID))
		h.emit(HubEvent{
			Type: HubEventRoomOffline,
			Payload: RoomOfflinePayload{
				RoomID:   payload.RoomID,
				SenderFP: msg.Sender.Fingerprint,
				Message:  msg,
			},
		})
		return
	}

	if !h.isUserInRoom(msg.Sender, targetRoom) {
		h.logg.Debug("message not sent", zap.String("error", "user not in room"), zap.String("fingerprint", msg.Sender.Username))
		return
	}

	data, err := json.Marshal(msg)
	if err != nil {
		h.logg.Error("failed to marshal message", zap.Error(err))
		return
	}
	h.sendMessageToRoom(targetRoom, data)

	h.emit(HubEvent{
		Type: HubEventMessageCreated,
		Payload: MessageCreatedPayload{
			ID:         uuid.NewString(),
			RoomID:     payload.RoomID,
			SenderFP:   msg.Sender.Fingerprint,
			Epoch:      int64(payload.Epoch),
			Ciphertext: payload.Ciphertext,
			CreatedAt:  time.Now().Unix(),
		},
	})
}

func (h *Hub) handleRegisterRoom(req CreateRoomRequest) {
	roomHost := h.getUser(req.Host.PublicKeyFingerprint)
	if roomHost == nil {
		h.logg.Debug("host user not found", zap.String("sessionId", req.Host.SessionID))
		req.ResponseChan <- ""
		return
	}
	roomID := uuid.NewString()
	h.Rooms[roomID] = &Room{
		Name:     req.GroupName,
		RoomID:   roomID,
		Host:     roomHost,
		HostFP:   roomHost.Fingerprint,
		HostName: roomHost.Username,
		Users:    map[string]*User{roomHost.Fingerprint: roomHost},
		Epoch:    0,
	}

	req.ResponseChan <- roomID
	h.logg.Debug("room created", zap.String("roomID", roomID), zap.String("host", req.Host.Username))

	h.emit(HubEvent{
		Type: HubEventRoomCreated,
		Payload: RoomCreatedPayload{
			RoomID:      roomID,
			Name:        req.GroupName,
			CreatorFP:   roomHost.Fingerprint,
			CreatorMode: roomHost.Mode,
			CreatedAt:   time.Now().Unix(),
		},
	})
}

func (h *Hub) handleUnregisterRoom(roomID string) {}

func (h *Hub) handleRegisterUser(user *User) {
	h.Users[user.Fingerprint] = user
	h.logg.Info("user connected", zap.String("username", user.Username), zap.String("fingerprint", user.Fingerprint))
}

// handleUnregisterUser removes the user from h.Users and from any hub rooms they were in.
// If a room becomes empty after removal it is deactivated (removed from h.Rooms) but kept in DB.
func (h *Hub) handleUnregisterUser(user *User) {
	delete(h.Users, user.Fingerprint)
	for _, room := range h.Rooms {
		if _, inRoom := room.Users[user.Fingerprint]; !inRoom {
			continue
		}
		delete(room.Users, user.Fingerprint)
		if len(room.Users) == 0 {
			delete(h.Rooms, room.RoomID)
		}
	}
	h.logg.Info("user disconnected", zap.String("username", user.Username), zap.String("fingerprint", user.Fingerprint))
}

// handleJoinRoom adds a user to a hub room and emits MemberJoined for DB persistence.
// Ownership verification is done at the HTTP handler level before sending this command.
func (h *Hub) handleJoinRoom(req RoomMembersChangeRequest) {
	room := h.getRoom(req.RoomID)
	if room == nil {
		h.logg.Debug("join room failed: room not found in hub", zap.String("roomID", req.RoomID))
		return
	}
	var memberMode models.UserMode
	if newUser := h.getUser(req.UserFP); newUser != nil {
		room.Users[newUser.Fingerprint] = newUser
		memberMode = newUser.Mode
		h.logg.Debug("user joined", zap.String("fingerprint", req.UserFP))
	}
	h.emit(HubEvent{
		Type: HubEventMemberJoined,
		Payload: MemberJoinedPayload{
			RoomID:     req.RoomID,
			MemberFP:   req.UserFP,
			MemberMode: memberMode,
			JoinedAt:   time.Now().Unix(),
		},
	})
}

// handleAddToRoom adds a live user to an existing hub room with no event and no DB change.
// Used on WS connect (Case 1) to slot the user into rooms already active in the hub.
func (h *Hub) handleAddToRoom(req AddToRoomRequest) {
	if room := h.getRoom(req.RoomID); room != nil {
		room.Users[req.User.Fingerprint] = req.User
		h.logg.Debug("user added to active room on connect", zap.String("roomID", req.RoomID), zap.String("user", req.User.Fingerprint))
	}
}

// handleRestoreRoom creates a room entry in h.Rooms if it doesn't exist, then adds all
// currently online members. Used when activating an offline room (Case 3 HTTP add-member).
func (h *Hub) handleRestoreRoom(req RestoreRoomRequest) {
	room, exists := h.Rooms[req.RoomID]
	if !exists {
		hostPtr := h.getUser(req.HostFP)
		hostName := req.HostFP // fallback to FP when name unavailable
		if hostPtr != nil {
			hostName = hostPtr.Username
		}
		room = &Room{
			Name:     req.RoomName,
			RoomID:   req.RoomID,
			Host:     hostPtr,
			HostFP:   req.HostFP,
			HostName: hostName,
			Users:    make(map[string]*User),
			Epoch:    req.Epoch,
		}
		h.Rooms[req.RoomID] = room
		h.logg.Debug("room activated into hub", zap.String("roomID", req.RoomID))
	}
	for _, fp := range req.MemberFPs {
		if u := h.getUser(fp); u != nil {
			room.Users[u.Fingerprint] = u
		}
	}
}

// handleLoadRoomAndDeliver creates a room (same as handleRestoreRoom) and then routes
// the pending message. Used by the event consumer for Case 2 (message to offline room).
func (h *Hub) handleLoadRoomAndDeliver(req LoadRoomAndDeliverRequest) {
	room, exists := h.Rooms[req.RoomID]
	if !exists {
		hostPtr := h.getUser(req.HostFP)
		hostName := req.HostFP
		if hostPtr != nil {
			hostName = hostPtr.Username
		}
		room = &Room{
			Name:     req.RoomName,
			RoomID:   req.RoomID,
			Host:     hostPtr,
			HostFP:   req.HostFP,
			HostName: hostName,
			Users:    make(map[string]*User),
			Epoch:    req.Epoch,
		}
		h.Rooms[req.RoomID] = room
		h.logg.Debug("room loaded into hub for message delivery", zap.String("roomID", req.RoomID))
	}
	for _, fp := range req.MemberFPs {
		if u := h.getUser(fp); u != nil {
			room.Users[u.Fingerprint] = u
		}
	}

	// Route the pending message
	if !h.isUserInRoom(req.Message.Sender, room) {
		h.logg.Debug("pending message sender not in room, dropping", zap.String("roomID", req.RoomID))
		return
	}
	data, err := json.Marshal(req.Message)
	if err != nil {
		h.logg.Error("failed to marshal pending message", zap.Error(err))
		return
	}
	h.sendMessageToRoom(room, data)

	var msgPayload MessagePayload
	if err := json.Unmarshal(req.Message.Payload, &msgPayload); err == nil {
		h.emit(HubEvent{
			Type: HubEventMessageCreated,
			Payload: MessageCreatedPayload{
				ID:         uuid.NewString(),
				RoomID:     req.RoomID,
				SenderFP:   req.Message.Sender.Fingerprint,
				Epoch:      int64(msgPayload.Epoch),
				Ciphertext: msgPayload.Ciphertext,
				CreatedAt:  time.Now().Unix(),
			},
		})
	}
}

// handleSendErrorToUser sends a WS error message to a specific online user.
func (h *Hub) handleSendErrorToUser(req SendErrorRequest) {
	u := h.getUser(req.UserFP)
	if u == nil {
		return
	}
	data, err := json.Marshal(map[string]any{"type": "error", "payload": map[string]string{"message": req.Message}})
	if err != nil {
		return
	}
	select {
	case u.OutGoingMessages <- data:
	default:
		h.logg.Warn("could not send error to user, outgoing channel full", zap.String("userFP", req.UserFP))
	}
}

// handleRemoveFromRoom removes a member from a hub room and emits the appropriate events.
// - Always emits MemberRemoved (for DB removal).
// - If room becomes empty and removed member was the host: also emits RoomDeleted (full DB deletion).
// - If room becomes empty and removed member was not the host: just deactivates room (keeps in DB).
// - If others remain and removed member was the host: transfers host randomly.
func (h *Hub) handleRemoveFromRoom(req RemoveFromRoomRequest) {
	room := h.getRoom(req.RoomID)
	if room == nil {
		return
	}

	wasHost := req.MemberFP == room.HostFP
	delete(room.Users, req.MemberFP)

	h.emit(HubEvent{
		Type:    HubEventMemberRemoved,
		Payload: MemberRemovedPayload{RoomID: req.RoomID, MemberFP: req.MemberFP},
	})

	if len(room.Users) == 0 {
		delete(h.Rooms, req.RoomID)
		if wasHost {
			h.emit(HubEvent{
				Type:    HubEventRoomDeleted,
				Payload: RoomDeletedPayload{RoomID: req.RoomID},
			})
		}
		return
	}

	if wasHost {
		h.transferHostRandom(room)
	}
}

// transferHostRandom picks a random active member as the new host, updates the room, and emits HostTransferred.
func (h *Hub) transferHostRandom(room *Room) {
	for _, u := range room.Users {
		room.Host = u
		room.HostFP = u.Fingerprint
		room.HostName = u.Username
		h.emit(HubEvent{
			Type:    HubEventHostTransferred,
			Payload: HostTransferredPayload{RoomID: room.RoomID, NewHostFP: u.Fingerprint},
		})
		h.logg.Debug("host transferred", zap.String("roomID", room.RoomID), zap.String("newHost", u.Fingerprint))
		return
	}
}
