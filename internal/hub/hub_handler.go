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
	case models.KeyRotationUpdate:
		h.handleKeyRotationUpdate(msg)
	case models.Signal:
		h.handleSignal(msg)
	case models.VoiceJoined:
		h.handleVoiceJoined(msg)
	case models.VoiceLeft:
		h.handleVoiceLeft(msg)
	default:
		h.logg.Debug("ignoring unhandled inbound envelope type",
			zap.String("type", string(msg.Type)),
			zap.String("user", msg.Sender.Fingerprint),
		)
	}
}

func (h *Hub) handleMessageEnvelope(msg WSMessageEnvelope) {
	if !msg.Sender.msgLimiter.Allow() {
		h.logg.Warn("chat message rate limit exceeded, dropping",
			zap.String("username", msg.Sender.Username),
			zap.String("fingerprint", msg.Sender.Fingerprint),
		)
		return
	}

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
// MemberPublicKeys and MemberModes are intentionally NOT deleted on disconnect so that
// the member is included in future rotation payloads and can be re-slotted on reconnect.
// They are removed only on explicit leave (handleRemoveFromRoom).
func (h *Hub) handleUnregisterUser(user *User) {
	delete(h.Users, user.Fingerprint)
	for _, room := range h.Rooms {
		if _, inRoom := room.Users[user.Fingerprint]; !inRoom {
			continue
		}
		delete(room.Users, user.Fingerprint)

		// Voice cleanup: auto-broadcast voice_left if disconnecting user was in voice.
		// Must run before the room-offline check; sendMessageToRoom needs room.Users.
		if room.VoiceUsers != nil {
			if _, wasInVoice := room.VoiceUsers[user.Fingerprint]; wasInVoice {
				delete(room.VoiceUsers, user.Fingerprint)
				h.broadcastVoiceLeft(room, user.Fingerprint)
			}
		}

		// Host transfer: only when an ephemeral host disconnects.
		// Persistent hosts retain ownership while offline and resume on reconnect.
		if room.HostFP == user.Fingerprint && user.Mode == models.UserModeEphemeral {
			newHostFP := h.selectNextByLex(room, user.Fingerprint)
			room.HostFP = newHostFP
			if newHostFP != "" {
				if newHostPtr := h.getUser(newHostFP); newHostPtr != nil {
					room.Host = newHostPtr
					room.HostName = newHostPtr.Username
				} else {
					room.Host = nil
					// HostName left stale; will be corrected on next RestoreRoom.
				}
			} else {
				room.Host = nil
				room.HostName = ""
			}
			h.emit(HubEvent{
				Type:    HubEventHostTransferred,
				Payload: HostTransferredPayload{RoomID: room.RoomID, NewHostFP: newHostFP},
			})
			h.logg.Info("group owner changed (host disconnect)",
				zap.String("roomID", room.RoomID),
				zap.String("oldHostFP", user.Fingerprint),
				zap.String("oldHostUsername", user.Username),
				zap.String("newHostFP", newHostFP),
				zap.String("newHostUsername", room.HostName),
			)
		}

		if len(room.Users) == 0 {
			delete(h.Rooms, room.RoomID)
			h.logg.Info("room went offline (last user disconnected)", zap.String("roomID", room.RoomID), zap.String("roomName", room.Name), zap.Int("activeRooms", len(h.Rooms)))
			continue
		}

		// Notify remaining online members that the room state changed.
		h.emit(HubEvent{
			Type:    HubEventRoomUpdated,
			Payload: RoomUpdatedEventPayload{RoomID: room.RoomID},
		})

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
	// The added user is already in room.Users at this point, so they receive this too.
	h.broadcastMemberJoined(room, req.UserFP, req.UserMode)
}

// handleAddToRoom adds a live user to an existing hub room with no event and no DB change.
// Used on WS connect (Case 1) to slot the user into rooms already active in the hub.
// If there is a pending rotation:
//   - and this user IS the key creator: re-sends the rotation WS directly to them.
//   - and this user is a regular member: re-signals the key creator so they can include
//     the newly online member in their key submission.
func (h *Hub) handleAddToRoom(req AddToRoomRequest) {
	room := h.getRoom(req.RoomID)
	if room == nil {
		return
	}
	room.Users[req.User.Fingerprint] = req.User
	room.MemberPublicKeys[req.User.Fingerprint] = req.User.X25519PublicKey
	room.MemberModes[req.User.Fingerprint] = req.User.Mode
	h.logg.Debug("user added to active room on connect", zap.String("roomID", req.RoomID), zap.String("user", req.User.Fingerprint))

	if room.PendingRotation != nil {
		if room.PendingRotation.KeyCreatorFP == req.User.Fingerprint {
			// This user is the pending key creator — resend the rotation WS to them.
			h.sendRotationToUser(req.User, room)
		} else if creator := h.getUser(room.PendingRotation.KeyCreatorFP); creator != nil {
			// A regular member came online mid-rotation — re-signal the key creator so
			// they include this member in their encrypted key submission.
			h.sendRotationToUser(creator, room)
		}
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

	// Notify all online members (including the removed user) before evicting them.
	h.broadcastMemberLeft(room, req.MemberFP)

	delete(room.Users, req.MemberFP)
	delete(room.MemberPublicKeys, req.MemberFP)
	delete(room.MemberModes, req.MemberFP)
	h.logg.Debug("member removed from room in hub", zap.String("roomID", req.RoomID), zap.String("memberFP", req.MemberFP), zap.Int("onlineCount", len(room.Users)))

	// Update host if it changed.
	if req.NewHostFP != "" && req.NewHostFP != room.HostFP {
		oldHostFP := room.HostFP
		room.HostFP = req.NewHostFP
		newHostUsername := req.NewHostFP // fallback to FP if user is offline
		if u := h.getUser(req.NewHostFP); u != nil {
			room.Host = u
			room.HostName = u.Username
			newHostUsername = u.Username
		} else {
			room.Host = nil
			room.HostName = req.NewHostFP
		}
		h.logg.Info("group owner changed (member removed)",
			zap.String("roomID", req.RoomID),
			zap.String("oldHostFP", oldHostFP),
			zap.String("newHostFP", req.NewHostFP),
			zap.String("newHostUsername", newHostUsername),
		)
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

// handleBroadcastToRoom sends pre-marshaled data to all online members of a room by ID.
// The caller is responsible for building and marshaling the full WS envelope.
func (h *Hub) handleBroadcastToRoom(req BroadcastToRoomRequest) {
	room := h.getRoom(req.RoomID)
	if room == nil {
		return
	}
	h.sendMessageToRoom(room, req.Data)
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
	// Only the key creator needs the rotation signal.
	creator := h.getUser(room.KeyCreatorFP)
	if creator != nil {
		h.sendToUser(creator, envelope)
	}

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

// handleSignal forwards a WebRTC signaling message (offer/answer/ICE) from one peer to another.
// Both sender and target must be members of the same room.
func (h *Hub) handleSignal(msg WSMessageEnvelope) {
	var payload SignalPayloadInbound
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		h.logg.Debug("handleSignal: failed to unmarshal payload", zap.Error(err))
		return
	}

	room := h.getRoom(payload.RoomID)
	if room == nil {
		h.logg.Debug("handleSignal: room not in hub", zap.String("roomID", payload.RoomID))
		return
	}
	if !h.isUserInRoom(msg.Sender, room) {
		h.logg.Warn("handleSignal: sender not in room", zap.String("roomID", payload.RoomID), zap.String("senderFP", msg.Sender.Fingerprint))
		return
	}
	target := h.getUser(payload.TargetFP)
	if target == nil {
		h.logg.Debug("handleSignal: target not online", zap.String("targetFP", payload.TargetFP))
		return
	}
	if !h.isUserInRoom(target, room) {
		h.logg.Warn("handleSignal: target not in same room", zap.String("roomID", payload.RoomID), zap.String("targetFP", payload.TargetFP))
		return
	}

	outPayload, err := json.Marshal(SignalPayloadOutbound{
		RoomID: payload.RoomID,
		FromFP: msg.Sender.Fingerprint,
		Signal: payload.Signal,
	})
	if err != nil {
		h.logg.Error("handleSignal: failed to marshal outbound payload", zap.Error(err))
		return
	}
	envelope, err := json.Marshal(WSMessageEnvelope{
		Type:    models.Signal,
		Payload: json.RawMessage(outPayload),
	})
	if err != nil {
		h.logg.Error("handleSignal: failed to marshal envelope", zap.Error(err))
		return
	}
	h.sendToUser(target, envelope)
	h.logg.Debug("signal forwarded", zap.String("roomID", payload.RoomID), zap.String("from", msg.Sender.Fingerprint), zap.String("to", payload.TargetFP))
}

// handleVoiceJoined tracks a user joining voice, replies with current voice participants,
// and broadcasts the join to all other room members.
func (h *Hub) handleVoiceJoined(msg WSMessageEnvelope) {
	var payload VoiceJoinedPayloadInbound
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		h.logg.Debug("handleVoiceJoined: failed to unmarshal payload", zap.Error(err))
		return
	}

	room := h.getRoom(payload.RoomID)
	if room == nil {
		h.logg.Debug("handleVoiceJoined: room not in hub", zap.String("roomID", payload.RoomID))
		return
	}
	if !h.isUserInRoom(msg.Sender, room) {
		h.logg.Warn("handleVoiceJoined: sender not in room", zap.String("roomID", payload.RoomID), zap.String("senderFP", msg.Sender.Fingerprint))
		return
	}

	if room.VoiceUsers == nil {
		room.VoiceUsers = make(map[string]struct{})
	}

	// Collect existing voice FPs before adding the new joiner.
	existingFPs := make([]string, 0, len(room.VoiceUsers))
	for fp := range room.VoiceUsers {
		existingFPs = append(existingFPs, fp)
	}
	room.VoiceUsers[msg.Sender.Fingerprint] = struct{}{}

	// Reply to the joiner with the list of existing voice participants.
	joinerPayload, err := json.Marshal(VoiceJoinedPayloadOutbound{
		RoomID:      payload.RoomID,
		Fingerprint: msg.Sender.Fingerprint,
		VoiceUsers:  existingFPs,
	})
	if err != nil {
		h.logg.Error("handleVoiceJoined: failed to marshal joiner payload", zap.Error(err))
		return
	}
	joinerEnvelope, err := json.Marshal(WSMessageEnvelope{
		Type:    models.VoiceJoined,
		Payload: json.RawMessage(joinerPayload),
	})
	if err != nil {
		h.logg.Error("handleVoiceJoined: failed to marshal joiner envelope", zap.Error(err))
		return
	}
	h.sendToUser(msg.Sender, joinerEnvelope)

	// Broadcast to all other room members (no voice_users field).
	broadcastPayload, err := json.Marshal(VoiceJoinedPayloadOutbound{
		RoomID:      payload.RoomID,
		Fingerprint: msg.Sender.Fingerprint,
	})
	if err != nil {
		h.logg.Error("handleVoiceJoined: failed to marshal broadcast payload", zap.Error(err))
		return
	}
	broadcastEnvelope, err := json.Marshal(WSMessageEnvelope{
		Type:    models.VoiceJoined,
		Payload: json.RawMessage(broadcastPayload),
	})
	if err != nil {
		h.logg.Error("handleVoiceJoined: failed to marshal broadcast envelope", zap.Error(err))
		return
	}
	for fp, u := range room.Users {
		if fp == msg.Sender.Fingerprint {
			continue
		}
		h.sendToUser(u, broadcastEnvelope)
	}

	h.logg.Info("voice joined", zap.String("roomID", payload.RoomID), zap.String("userFP", msg.Sender.Fingerprint), zap.Int("existingVoiceUsers", len(existingFPs)))
}

// handleVoiceLeft removes a user from voice state and broadcasts the leave to the room.
func (h *Hub) handleVoiceLeft(msg WSMessageEnvelope) {
	var payload VoiceLeftPayloadInbound
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		h.logg.Debug("handleVoiceLeft: failed to unmarshal payload", zap.Error(err))
		return
	}

	room := h.getRoom(payload.RoomID)
	if room == nil {
		h.logg.Debug("handleVoiceLeft: room not in hub", zap.String("roomID", payload.RoomID))
		return
	}
	if !h.isUserInRoom(msg.Sender, room) {
		h.logg.Warn("handleVoiceLeft: sender not in room", zap.String("roomID", payload.RoomID), zap.String("senderFP", msg.Sender.Fingerprint))
		return
	}
	if room.VoiceUsers == nil {
		return
	}
	if _, wasInVoice := room.VoiceUsers[msg.Sender.Fingerprint]; !wasInVoice {
		return
	}

	delete(room.VoiceUsers, msg.Sender.Fingerprint)
	h.broadcastVoiceLeft(room, msg.Sender.Fingerprint)
	h.logg.Info("voice left", zap.String("roomID", payload.RoomID), zap.String("userFP", msg.Sender.Fingerprint))
}

// broadcastVoiceLeft notifies all online room members that a user has left voice.
func (h *Hub) broadcastVoiceLeft(room *Room, fp string) {
	payload, err := json.Marshal(VoiceLeftPayloadOutbound{RoomID: room.RoomID, Fingerprint: fp})
	if err != nil {
		h.logg.Error("broadcastVoiceLeft: failed to marshal payload", zap.Error(err))
		return
	}
	envelope, err := json.Marshal(WSMessageEnvelope{Type: models.VoiceLeft, Payload: json.RawMessage(payload)})
	if err != nil {
		h.logg.Error("broadcastVoiceLeft: failed to marshal envelope", zap.Error(err))
		return
	}
	h.sendMessageToRoom(room, envelope)
}

// sendToUser delivers a message to a specific user's outgoing channel non-blocking.
func (h *Hub) sendToUser(u *User, data []byte) {
	select {
	case u.OutGoingMessages <- data:
	default:
		h.logg.Warn("outgoing channel full, dropping message to user", zap.String("userFP", u.Fingerprint))
	}
}

// handleKeyRotationUpdate processes a room_key_rotation_update WS message from the key creator.
// Validates the sender is the current key creator, decodes the encrypted keys, delivers them to
// online members immediately, and emits an event for DB persistence.
func (h *Hub) handleKeyRotationUpdate(msg WSMessageEnvelope) {
	var raw struct {
		RoomID string `json:"room_id"`
		Epoch  int64  `json:"epoch"`
		Keys   []struct {
			RecipientFP  string `json:"recipient_fp"`
			EncryptedKey string `json:"encrypted_key"`
		} `json:"keys"`
	}
	if err := json.Unmarshal(msg.Payload, &raw); err != nil {
		h.logg.Debug("handleKeyRotationUpdate: failed to unmarshal payload", zap.Error(err))
		return
	}

	room := h.getRoom(raw.RoomID)
	if room == nil {
		h.logg.Debug("handleKeyRotationUpdate: room not in hub", zap.String("roomID", raw.RoomID))
		return
	}

	if room.PendingRotation == nil {
		h.logg.Debug("handleKeyRotationUpdate: no pending rotation", zap.String("roomID", raw.RoomID))
		return
	}
	if room.PendingRotation.KeyCreatorFP != msg.Sender.Fingerprint {
		h.logg.Warn("handleKeyRotationUpdate: sender is not the key creator",
			zap.String("roomID", raw.RoomID),
			zap.String("senderFP", msg.Sender.Fingerprint),
			zap.String("keyCreatorFP", room.PendingRotation.KeyCreatorFP),
		)
		return
	}
	if room.PendingRotation.Epoch != raw.Epoch {
		h.logg.Debug("handleKeyRotationUpdate: epoch mismatch",
			zap.String("roomID", raw.RoomID),
			zap.Int64("pendingEpoch", room.PendingRotation.Epoch),
			zap.Int64("receivedEpoch", raw.Epoch),
		)
		return
	}

	entries := make([]KeySlotEntry, 0, len(raw.Keys))
	for _, k := range raw.Keys {
		decoded, err := base64.StdEncoding.DecodeString(k.EncryptedKey)
		if err != nil {
			h.logg.Debug("handleKeyRotationUpdate: failed to decode key for recipient", zap.String("recipientFP", k.RecipientFP), zap.Error(err))
			continue
		}
		entries = append(entries, KeySlotEntry{RecipientFP: k.RecipientFP, EncryptedKey: decoded})
	}

	// Deliver to online members and clear PendingRotation.
	h.handleEpochKeysSubmitted(EpochKeysSubmittedRequest{
		RoomID: raw.RoomID,
		Epoch:  raw.Epoch,
		Keys:   entries,
	})

	// Persist key slots via event consumer.
	h.emit(HubEvent{
		Type: HubEventEpochKeysSubmitted,
		Payload: EpochKeysSubmittedPayload{
			RoomID: raw.RoomID,
			Epoch:  raw.Epoch,
			Keys:   entries,
		},
	})

	h.logg.Info("key rotation update processed",
		zap.String("roomID", raw.RoomID),
		zap.Int64("epoch", raw.Epoch),
		zap.Int("keyCount", len(entries)),
	)
}

func (h *Hub) handlePurgeUserMessages(req PurgeRequest) {
	for _, room := range h.Rooms {
		if _, isMember := room.MemberPublicKeys[req.SenderFP]; !isMember {
			continue
		}
		payload, err := json.Marshal(MessagesPurgedPayload{
			SenderFP: req.SenderFP,
			RoomID:   room.RoomID,
		})
		if err != nil {
			h.logg.Error("purge: failed to marshal payload", zap.String("roomID", room.RoomID), zap.Error(err))
			continue
		}
		data, err := json.Marshal(outboundEnvelope{
			SenderFP: req.SenderFP,
			Type:     models.MessagesPurged,
			Payload:  payload,
		})
		if err != nil {
			h.logg.Error("purge: failed to marshal envelope", zap.String("roomID", room.RoomID), zap.Error(err))
			continue
		}
		h.sendMessageToRoom(room, data)
	}
}
