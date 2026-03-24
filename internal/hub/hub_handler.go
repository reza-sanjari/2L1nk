package hub

import (
	"2L1nk/internal/models"
	"encoding/base64"
	"encoding/json"
	"sort"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

func (h *Hub) handleInboundMessage(msg WSMessageEnvelope) {
	switch msg.Type {
	case models.Message:
		h.handleMessageEnvelope(msg)
	case models.Auth:
		h.handleReAuth(msg)
	default:
		h.logg.Debug("ignoring unhandled inbound envelope type",
			zap.String("type", string(msg.Type)),
			zap.String("user", msg.Sender.Fingerprint),
		)
	}
}

func (h *Hub) handleMessageEnvelope(msg WSMessageEnvelope) {
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

	// Reject messages encrypted with a stale epoch key.
	if int64(payload.Epoch) != targetRoom.Epoch {
		errPayload, _ := json.Marshal(EpochMismatchPayload{
			RoomID:       payload.RoomID,
			CurrentEpoch: targetRoom.Epoch,
		})
		envelope, _ := json.Marshal(WSMessageEnvelope{
			Type:    models.EpochMismatch,
			Payload: json.RawMessage(errPayload),
		})
		h.sendToUser(msg.Sender, envelope)
		h.logg.Debug("epoch mismatch, message rejected",
			zap.String("roomID", payload.RoomID),
			zap.Int64("currentEpoch", targetRoom.Epoch),
			zap.Uint64("receivedEpoch", payload.Epoch),
		)
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

func (h *Hub) handleReAuth(msg WSMessageEnvelope) {
	// User is already registered in the hub (ReadPump only runs for authenticated users).
	// A duplicate auth message is ignored for now.
	h.logg.Debug("re-auth envelope received, ignoring",
		zap.String("user", msg.Sender.Fingerprint),
	)
}

func (h *Hub) handleRegisterRoom(req CreateRoomRequest) {
	roomHost := h.getUser(req.Host.PublicKeyFingerprint)
	if roomHost == nil {
		h.logg.Debug("host user not found", zap.String("sessionId", req.Host.SessionID))
		req.ResponseChan <- ""
		return
	}
	roomID := uuid.NewString()
	room := &Room{
		Name:             req.GroupName,
		RoomID:           roomID,
		Host:             roomHost,
		HostFP:           roomHost.Fingerprint,
		HostName:         roomHost.Username,
		KeyCreatorFP:     roomHost.Fingerprint,
		Users:            map[string]*User{roomHost.Fingerprint: roomHost},
		Epoch:            0,
		MemberPublicKeys: map[string]string{roomHost.Fingerprint: roomHost.X25519PublicKey},
		MemberModes:      map[string]models.UserMode{roomHost.Fingerprint: roomHost.Mode},
	}
	h.Rooms[roomID] = room

	req.ResponseChan <- roomID
	h.logg.Info("room created", zap.String("roomID", roomID), zap.String("host", req.Host.Username), zap.String("name", req.GroupName), zap.Int("activeRooms", len(h.Rooms)))

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

	// Broadcast epoch 0 rotation so the host can generate and submit the initial room key.
	// emitEvent=true: the event consumer will persist epoch 0 + key creator.
	h.broadcastRotation(room, true)
}

func (h *Hub) handleRegisterUser(user *User) {
	h.Users[user.Fingerprint] = user
	h.logg.Info("user connected", zap.String("username", user.Username), zap.String("fingerprint", user.Fingerprint), zap.Int("activeUsers", len(h.Users)))
}

// handleUnregisterUser removes the user from h.Users and from any hub rooms they were in.
// If a room becomes empty it is removed from h.Rooms (auto-offline) but kept in DB.
// If the user was the pending key creator, a new online key creator is selected and
// the rotation is re-broadcast for the same epoch (event emitted for DB persistence).
func (h *Hub) handleUnregisterUser(user *User) {
	delete(h.Users, user.Fingerprint)
	for _, room := range h.Rooms {
		if _, inRoom := room.Users[user.Fingerprint]; !inRoom {
			continue
		}
		delete(room.Users, user.Fingerprint)
		delete(room.MemberPublicKeys, user.Fingerprint)
		delete(room.MemberModes, user.Fingerprint)

		if len(room.Users) == 0 {
			delete(h.Rooms, room.RoomID)
			h.logg.Info("room went offline (last user disconnected)", zap.String("roomID", room.RoomID), zap.String("roomName", room.Name), zap.Int("activeRooms", len(h.Rooms)))
			continue
		}

		// Re-assign key creator if this user was the pending key creator.
		if room.PendingRotation != nil && room.PendingRotation.KeyCreatorFP == user.Fingerprint {
			newCreator := h.selectNextByLex(room, user.Fingerprint)
			if newCreator != "" {
				room.KeyCreatorFP = newCreator
				h.logg.Debug("key creator reassigned on disconnect",
					zap.String("roomID", room.RoomID),
					zap.String("newCreator", newCreator),
				)
				h.broadcastRotation(room, true) // same epoch, emit event for DB update
			}
		}
	}
	h.logg.Info("user disconnected", zap.String("username", user.Username), zap.String("fingerprint", user.Fingerprint), zap.Int("activeUsers", len(h.Users)))
}

// handleJoinRoom syncs hub state after a DB-first member add.
// The DB has already been updated; this just updates hub state and broadcasts the rotation WS.
func (h *Hub) handleJoinRoom(req RoomMembersChangeRequest) {
	room := h.getRoom(req.RoomID)
	if room == nil {
		// Room is offline — nothing to sync. DB is already updated.
		h.logg.Debug("join room hub sync skipped: room not in hub", zap.String("roomID", req.RoomID))
		return
	}

	// Add to MemberPublicKeys and MemberModes regardless of online status.
	if req.UserX25519Key != "" {
		room.MemberPublicKeys[req.UserFP] = req.UserX25519Key
	}
	room.MemberModes[req.UserFP] = req.UserMode

	if newUser := h.getUser(req.UserFP); newUser != nil {
		room.Users[newUser.Fingerprint] = newUser
		room.MemberPublicKeys[newUser.Fingerprint] = newUser.X25519PublicKey // prefer live key
		h.logg.Debug("member joined room (online)", zap.String("roomID", req.RoomID), zap.String("memberFP", req.UserFP), zap.Int("onlineCount", len(room.Users)))
	} else {
		h.logg.Debug("member added to room (offline)", zap.String("roomID", req.RoomID), zap.String("memberFP", req.UserFP))
	}

	prevCreator := room.KeyCreatorFP
	room.Epoch = req.NewEpoch
	room.KeyCreatorFP = req.NewKeyCreatorFP
	if prevCreator != req.NewKeyCreatorFP {
		h.logg.Debug("key creator changed on member join", zap.String("roomID", req.RoomID), zap.String("from", prevCreator), zap.String("to", req.NewKeyCreatorFP), zap.Int64("epoch", req.NewEpoch))
	}

	// Broadcast rotation WS; DB already updated so no event emission.
	h.broadcastRotation(room, false)
	// Notify all online members that a new member has joined.
	h.broadcastMemberJoined(room, req.UserFP, req.UserMode)
}

// handleAddToRoom adds a live user to an existing hub room with no event and no DB change.
// Used on WS connect (Case 1) to slot the user into rooms already active in the hub.
// If the user is the pending key creator, re-sends the rotation WS to them.
func (h *Hub) handleAddToRoom(req AddToRoomRequest) {
	room := h.getRoom(req.RoomID)
	if room == nil {
		return
	}
	room.Users[req.User.Fingerprint] = req.User
	room.MemberPublicKeys[req.User.Fingerprint] = req.User.X25519PublicKey
	room.MemberModes[req.User.Fingerprint] = req.User.Mode
	h.logg.Debug("user added to active room on connect", zap.String("roomID", req.RoomID), zap.String("user", req.User.Fingerprint))

	// If this user is the pending key creator, resend the rotation WS to them.
	if room.PendingRotation != nil && room.PendingRotation.KeyCreatorFP == req.User.Fingerprint {
		h.sendRotationToUser(req.User, room)
	}
}

// handleRestoreRoom creates a room entry in h.Rooms if it doesn't exist, then adds all
// currently online members. If HasPendingRotation is set and the key creator is now online,
// broadcasts the rotation WS so they can submit the pending epoch key.
func (h *Hub) handleRestoreRoom(req RestoreRoomRequest) {
	room, exists := h.Rooms[req.RoomID]
	if !exists {
		hostPtr := h.getUser(req.HostFP)
		hostName := req.HostFP
		if hostPtr != nil {
			hostName = hostPtr.Username
		}
		room = &Room{
			Name:             req.RoomName,
			RoomID:           req.RoomID,
			Host:             hostPtr,
			HostFP:           req.HostFP,
			HostName:         hostName,
			KeyCreatorFP:     req.KeyCreatorFP,
			Users:            make(map[string]*User),
			Epoch:            req.Epoch,
			MemberPublicKeys: make(map[string]string),
			MemberModes:      make(map[string]models.UserMode),
		}
		h.Rooms[req.RoomID] = room
		h.logg.Info("room restored into hub", zap.String("roomID", req.RoomID), zap.String("roomName", req.RoomName), zap.Int("activeRooms", len(h.Rooms)))
	}
	for _, m := range req.Members {
		room.MemberPublicKeys[m.FP] = m.X25519PublicKey
		room.MemberModes[m.FP] = m.Mode
		if u := h.getUser(m.FP); u != nil {
			room.Users[u.Fingerprint] = u
			room.MemberPublicKeys[u.Fingerprint] = u.X25519PublicKey // prefer live key
		}
	}
	h.logg.Debug("room restored: member slots populated", zap.String("roomID", req.RoomID), zap.Int("totalMembers", len(req.Members)), zap.Int("onlineMembers", len(room.Users)), zap.Bool("hasPendingRotation", req.HasPendingRotation))

	if req.HasPendingRotation && h.getUser(req.KeyCreatorFP) != nil {
		// Key creator is now online — broadcast rotation so they can submit the epoch key.
		// DB is already correct (epoch and key_creator_fp set), so no event needed.
		h.broadcastRotation(room, false)
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
			Name:             req.RoomName,
			RoomID:           req.RoomID,
			Host:             hostPtr,
			HostFP:           req.HostFP,
			HostName:         hostName,
			KeyCreatorFP:     req.HostFP,
			Users:            make(map[string]*User),
			Epoch:            req.Epoch,
			MemberPublicKeys: make(map[string]string),
			MemberModes:      make(map[string]models.UserMode),
		}
		h.Rooms[req.RoomID] = room
		h.logg.Debug("room loaded into hub for message delivery", zap.String("roomID", req.RoomID))
	}
	for _, m := range req.Members {
		room.MemberPublicKeys[m.FP] = m.X25519PublicKey
		room.MemberModes[m.FP] = m.Mode
		if u := h.getUser(m.FP); u != nil {
			room.Users[u.Fingerprint] = u
			room.MemberPublicKeys[u.Fingerprint] = u.X25519PublicKey
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

// handleRemoveFromRoom syncs hub state after a DB-first member remove.
// The DB has already been updated; this just updates hub state and optionally broadcasts rotation.
func (h *Hub) handleRemoveFromRoom(req RemoveFromRoomRequest) {
	room := h.getRoom(req.RoomID)
	if room == nil {
		return
	}

	if req.Deleted {
		delete(h.Rooms, req.RoomID)
		h.logg.Info("room removed from hub (deleted)", zap.String("roomID", req.RoomID), zap.Int("activeRooms", len(h.Rooms)))
		return
	}

	delete(room.Users, req.MemberFP)
	delete(room.MemberPublicKeys, req.MemberFP)
	delete(room.MemberModes, req.MemberFP)
	h.logg.Debug("member removed from room in hub", zap.String("roomID", req.RoomID), zap.String("memberFP", req.MemberFP), zap.Int("onlineCount", len(room.Users)))

	// Update host if it changed.
	if req.NewHostFP != "" && req.NewHostFP != room.HostFP {
		h.logg.Debug("host transferred in hub", zap.String("roomID", req.RoomID), zap.String("from", room.HostFP), zap.String("to", req.NewHostFP))
		room.HostFP = req.NewHostFP
		if u := h.getUser(req.NewHostFP); u != nil {
			room.Host = u
			room.HostName = u.Username
		} else {
			room.Host = nil
			room.HostName = req.NewHostFP
		}
	}

	prevCreator := room.KeyCreatorFP
	room.Epoch = req.NewEpoch
	room.KeyCreatorFP = req.NewKeyCreatorFP
	if prevCreator != req.NewKeyCreatorFP {
		h.logg.Debug("key creator changed on member remove", zap.String("roomID", req.RoomID), zap.String("from", prevCreator), zap.String("to", req.NewKeyCreatorFP), zap.Int64("epoch", req.NewEpoch))
	}

	if len(room.Users) == 0 {
		delete(h.Rooms, req.RoomID)
		h.logg.Info("room went offline (no online members after remove)", zap.String("roomID", req.RoomID), zap.Int("activeRooms", len(h.Rooms)))
		return
	}

	// Broadcast rotation WS; DB already updated so no event emission.
	h.broadcastRotation(room, false)
	// Notify remaining online members that a member has been removed.
	h.broadcastMemberLeft(room, req.MemberFP)
}

// selectNextByLex picks the next host or key creator using the priority:
// online persistent lex lowest → online ephemeral lex lowest →
// offline persistent lex lowest → offline ephemeral lex lowest.
// Excludes excludeFP. Uses room.Users for online status and room.MemberModes for mode info.
func (h *Hub) selectNextByLex(room *Room, excludeFP string) string {
	var onlinePersistent, onlineEphemeral, offlinePersistent, offlineEphemeral []string
	for fp, mode := range room.MemberModes {
		if fp == excludeFP {
			continue
		}
		if _, online := room.Users[fp]; online {
			if mode == models.UserModePersistent {
				onlinePersistent = append(onlinePersistent, fp)
			} else {
				onlineEphemeral = append(onlineEphemeral, fp)
			}
		} else {
			if mode == models.UserModePersistent {
				offlinePersistent = append(offlinePersistent, fp)
			} else {
				offlineEphemeral = append(offlineEphemeral, fp)
			}
		}
	}
	for _, bucket := range [][]string{onlinePersistent, onlineEphemeral, offlinePersistent, offlineEphemeral} {
		if len(bucket) > 0 {
			sort.Strings(bucket)
			return bucket[0]
		}
	}
	return ""
}

// handleBroadcast sends an arbitrary WS envelope to all online members of a room.
// Used by external callers (HTTP handlers, event consumer) for general-purpose room notifications.
func (h *Hub) handleBroadcast(req BroadcastRequest) {
	data, err := json.Marshal(req.Envelope)
	if err != nil {
		h.logg.Error("failed to marshal broadcast envelope", zap.Error(err))
		return
	}
	h.sendMessageToRoom(req.Room, data)
}

// broadcastMemberJoined notifies all online room members that a new member has joined.
func (h *Hub) broadcastMemberJoined(room *Room, fp string, mode models.UserMode) {
	payload, _ := json.Marshal(MemberJoinedPayload{RoomID: room.RoomID, FP: fp, Mode: mode})
	envelope, _ := json.Marshal(WSMessageEnvelope{Type: models.JoinRoom, Payload: json.RawMessage(payload)})
	h.sendMessageToRoom(room, envelope)
}

// broadcastMemberLeft notifies all online room members that a member has been removed.
func (h *Hub) broadcastMemberLeft(room *Room, fp string) {
	payload, _ := json.Marshal(MemberLeftPayload{RoomID: room.RoomID, FP: fp})
	envelope, _ := json.Marshal(WSMessageEnvelope{Type: models.LeaveRoom, Payload: json.RawMessage(payload)})
	h.sendMessageToRoom(room, envelope)
}

// broadcastRotation sets PendingRotation and sends a RoomKeyRotation WS message to all
// online members. Uses room.KeyCreatorFP as the key creator (caller must set it first).
// If emitEvent is true, emits HubEventKeyRotationTriggered for DB persistence.
func (h *Hub) broadcastRotation(room *Room, emitEvent bool) {
	room.PendingRotation = &PendingRotation{
		Epoch:        room.Epoch,
		KeyCreatorFP: room.KeyCreatorFP,
		TriggeredAt:  time.Now(),
	}

	// Refresh online member keys in the cache.
	for fp, u := range room.Users {
		room.MemberPublicKeys[fp] = u.X25519PublicKey
	}

	// Build full member list (online + offline persistent) from cache.
	members := make([]MemberWithKey, 0, len(room.MemberPublicKeys))
	for fp, key := range room.MemberPublicKeys {
		members = append(members, MemberWithKey{Fingerprint: fp, X25519PublicKey: key})
	}

	payloadBytes, err := json.Marshal(RoomKeyRotationPayload{
		RoomID:       room.RoomID,
		Epoch:        room.Epoch,
		KeyCreatorFP: room.KeyCreatorFP,
		Members:      members,
	})
	if err != nil {
		h.logg.Error("failed to marshal rotation payload", zap.Error(err))
		return
	}
	envelope, err := json.Marshal(WSMessageEnvelope{
		Type:    models.KeyRotation,
		Payload: json.RawMessage(payloadBytes),
	})
	if err != nil {
		h.logg.Error("failed to marshal rotation envelope", zap.Error(err))
		return
	}
	h.sendMessageToRoom(room, envelope)

	if emitEvent {
		h.emit(HubEvent{
			Type: HubEventKeyRotationTriggered,
			Payload: KeyRotationTriggeredPayload{
				RoomID:       room.RoomID,
				Epoch:        room.Epoch,
				KeyCreatorFP: room.KeyCreatorFP,
			},
		})
	}
}

// sendRotationToUser delivers a rotation WS message to one specific user.
// Used when the key creator reconnects mid-rotation.
func (h *Hub) sendRotationToUser(u *User, room *Room) {
	// Refresh online member keys.
	for fp, online := range room.Users {
		room.MemberPublicKeys[fp] = online.X25519PublicKey
	}
	members := make([]MemberWithKey, 0, len(room.MemberPublicKeys))
	for fp, key := range room.MemberPublicKeys {
		members = append(members, MemberWithKey{Fingerprint: fp, X25519PublicKey: key})
	}
	payloadBytes, err := json.Marshal(RoomKeyRotationPayload{
		RoomID:       room.RoomID,
		Epoch:        room.Epoch,
		KeyCreatorFP: room.KeyCreatorFP,
		Members:      members,
	})
	if err != nil {
		return
	}
	envelope, err := json.Marshal(WSMessageEnvelope{
		Type:    models.KeyRotation,
		Payload: json.RawMessage(payloadBytes),
	})
	if err != nil {
		return
	}
	h.sendToUser(u, envelope)
}

// handleEpochKeysSubmitted is called after the key creator submits encrypted keys via HTTP.
// Clears PendingRotation and delivers each key slot directly to online members via WebSocket.
func (h *Hub) handleEpochKeysSubmitted(req EpochKeysSubmittedRequest) {
	room := h.getRoom(req.RoomID)
	if room == nil {
		h.logg.Debug("epoch keys submitted but room not in hub, skipping delivery", zap.String("roomID", req.RoomID), zap.Int64("epoch", req.Epoch))
		return
	}
	room.PendingRotation = nil
	h.logg.Debug("pending rotation cleared", zap.String("roomID", req.RoomID), zap.Int64("epoch", req.Epoch))

	delivered := 0
	for _, entry := range req.Keys {
		u := h.getUser(entry.RecipientFP)
		if u == nil {
			continue
		}
		slotPayload, err := json.Marshal(RoomKeySlotPayload{
			RoomID:       req.RoomID,
			Epoch:        req.Epoch,
			EncryptedKey: base64.StdEncoding.EncodeToString(entry.EncryptedKey),
		})
		if err != nil {
			continue
		}
		envelope, err := json.Marshal(WSMessageEnvelope{
			Type:    models.KeySlot,
			Payload: json.RawMessage(slotPayload),
		})
		if err != nil {
			continue
		}
		h.sendToUser(u, envelope)
		delivered++
	}
	h.logg.Debug("epoch key slots delivered to online members", zap.String("roomID", req.RoomID), zap.Int64("epoch", req.Epoch), zap.Int("delivered", delivered), zap.Int("total", len(req.Keys)))
}

// sendToUser delivers a message to a specific user's outgoing channel non-blocking.
func (h *Hub) sendToUser(u *User, data []byte) {
	select {
	case u.OutGoingMessages <- data:
	default:
		h.logg.Warn("outgoing channel full, dropping message to user", zap.String("userFP", u.Fingerprint))
	}
}
