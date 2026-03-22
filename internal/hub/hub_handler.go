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
		Users:            map[string]*User{roomHost.Fingerprint: roomHost},
		Epoch:            0,
		MemberPublicKeys: map[string]string{roomHost.Fingerprint: roomHost.X25519PublicKey},
	}
	h.Rooms[roomID] = room

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

	// Broadcast epoch 0 rotation so the host can generate and submit the initial room key.
	h.broadcastRotation(room)
}

func (h *Hub) handleUnregisterRoom(roomID string) {}

func (h *Hub) handleRegisterUser(user *User) {
	h.Users[user.Fingerprint] = user
	h.logg.Info("user connected", zap.String("username", user.Username), zap.String("fingerprint", user.Fingerprint))
}

// handleUnregisterUser removes the user from h.Users and from any hub rooms they were in.
// If a room becomes empty after removal it is deactivated (removed from h.Rooms) but kept in DB.
// If the user was the pending key creator for any room, a new key creator is selected and the
// rotation event is re-broadcast (same epoch, no increment).
func (h *Hub) handleUnregisterUser(user *User) {
	delete(h.Users, user.Fingerprint)
	for _, room := range h.Rooms {
		if _, inRoom := room.Users[user.Fingerprint]; !inRoom {
			continue
		}
		delete(room.Users, user.Fingerprint)

		// Re-trigger rotation if this user was the pending key creator.
		if room.PendingRotation != nil && room.PendingRotation.KeyCreatorFP == user.Fingerprint && len(room.Users) > 0 {
			h.logg.Debug("key creator disconnected mid-rotation, re-triggering",
				zap.String("roomID", room.RoomID),
				zap.Int64("epoch", room.Epoch),
			)
			h.broadcastRotation(room) // same epoch, new key creator selected
		}

		if len(room.Users) == 0 {
			delete(h.Rooms, room.RoomID)
		}
	}
	h.logg.Info("user disconnected", zap.String("username", user.Username), zap.String("fingerprint", user.Fingerprint))
}

// handleJoinRoom adds a user to a hub room and emits MemberJoined for DB persistence.
// Ownership verification is done at the HTTP handler level before sending this command.
// Triggers key rotation so the new member gets the current epoch key.
func (h *Hub) handleJoinRoom(req RoomMembersChangeRequest) {
	room := h.getRoom(req.RoomID)
	if room == nil {
		h.logg.Debug("join room failed: room not found in hub", zap.String("roomID", req.RoomID))
		return
	}
	var memberMode models.UserMode
	if newUser := h.getUser(req.UserFP); newUser != nil {
		room.Users[newUser.Fingerprint] = newUser
		room.MemberPublicKeys[newUser.Fingerprint] = newUser.X25519PublicKey
		memberMode = newUser.Mode
		h.logg.Debug("user joined", zap.String("fingerprint", req.UserFP))
	} else {
		// User is offline but still gets a slot in MemberPublicKeys from a previous cache entry.
		// The key will be included in the rotation event for offline delivery via room_key_slots.
		h.logg.Debug("joining user is offline", zap.String("fingerprint", req.UserFP))
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
	h.triggerRotation(room)
}

// handleAddToRoom adds a live user to an existing hub room with no event and no DB change.
// Used on WS connect (Case 1) to slot the user into rooms already active in the hub.
func (h *Hub) handleAddToRoom(req AddToRoomRequest) {
	if room := h.getRoom(req.RoomID); room != nil {
		room.Users[req.User.Fingerprint] = req.User
		room.MemberPublicKeys[req.User.Fingerprint] = req.User.X25519PublicKey
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
			Name:             req.RoomName,
			RoomID:           req.RoomID,
			Host:             hostPtr,
			HostFP:           req.HostFP,
			HostName:         hostName,
			Users:            make(map[string]*User),
			Epoch:            req.Epoch,
			MemberPublicKeys: make(map[string]string),
		}
		h.Rooms[req.RoomID] = room
		h.logg.Debug("room activated into hub", zap.String("roomID", req.RoomID))
	}
	for _, m := range req.Members {
		room.MemberPublicKeys[m.FP] = m.X25519PublicKey
		if u := h.getUser(m.FP); u != nil {
			room.Users[u.Fingerprint] = u
			room.MemberPublicKeys[u.Fingerprint] = u.X25519PublicKey // prefer live key
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
			Name:             req.RoomName,
			RoomID:           req.RoomID,
			Host:             hostPtr,
			HostFP:           req.HostFP,
			HostName:         hostName,
			Users:            make(map[string]*User),
			Epoch:            req.Epoch,
			MemberPublicKeys: make(map[string]string),
		}
		h.Rooms[req.RoomID] = room
		h.logg.Debug("room loaded into hub for message delivery", zap.String("roomID", req.RoomID))
	}
	for _, m := range req.Members {
		room.MemberPublicKeys[m.FP] = m.X25519PublicKey
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

// handleRemoveFromRoom removes a member from a hub room and emits the appropriate events.
// - Always emits MemberRemoved (for DB removal).
// - If room becomes empty and removed member was the host: also emits RoomDeleted (full DB deletion).
// - If room becomes empty and removed member was not the host: just deactivates room (keeps in DB).
// - If others remain and removed member was the host: transfers host randomly.
// - Triggers key rotation so the removed member cannot decrypt future messages.
func (h *Hub) handleRemoveFromRoom(req RemoveFromRoomRequest) {
	room := h.getRoom(req.RoomID)
	if room == nil {
		return
	}

	wasHost := req.MemberFP == room.HostFP
	delete(room.Users, req.MemberFP)
	delete(room.MemberPublicKeys, req.MemberFP) // removed member must not receive the new key

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
	h.triggerRotation(room)
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

// selectKeyCreator returns the fingerprint of the member who should generate the next epoch key.
// Priority: host (if online) > lex lowest persistent online member > lex lowest ephemeral online member.
func (h *Hub) selectKeyCreator(room *Room) string {
	if _, ok := room.Users[room.HostFP]; ok {
		return room.HostFP
	}
	var persistentFPs []string
	for fp, u := range room.Users {
		if u.Mode == models.UserModePersistent {
			persistentFPs = append(persistentFPs, fp)
		}
	}
	if len(persistentFPs) > 0 {
		sort.Strings(persistentFPs)
		return persistentFPs[0]
	}
	var fps []string
	for fp := range room.Users {
		fps = append(fps, fp)
	}
	if len(fps) > 0 {
		sort.Strings(fps)
		return fps[0]
	}
	return ""
}

// triggerRotation increments the room epoch and broadcasts a key rotation event.
func (h *Hub) triggerRotation(room *Room) {
	room.Epoch++
	h.broadcastRotation(room)
}

// broadcastRotation selects a key creator, sets PendingRotation, and sends RoomKeyRotationEvent
// to all online members. Does NOT increment the epoch — call triggerRotation for that.
// Also emits HubEventKeyRotationTriggered for DB persistence.
func (h *Hub) broadcastRotation(room *Room) {
	keyCreator := h.selectKeyCreator(room)

	room.PendingRotation = &PendingRotation{
		Epoch:        room.Epoch,
		KeyCreatorFP: keyCreator,
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
		KeyCreatorFP: keyCreator,
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

	h.emit(HubEvent{
		Type: HubEventKeyRotationTriggered,
		Payload: KeyRotationTriggeredPayload{
			RoomID:       room.RoomID,
			Epoch:        room.Epoch,
			KeyCreatorFP: keyCreator,
		},
	})
}

// handleEpochKeysSubmitted is called after the key creator submits encrypted keys via HTTP.
// Clears PendingRotation and delivers each key slot directly to online members via WebSocket.
func (h *Hub) handleEpochKeysSubmitted(req EpochKeysSubmittedRequest) {
	room := h.getRoom(req.RoomID)
	if room == nil {
		return
	}
	room.PendingRotation = nil

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
	}
}

// sendToUser delivers a message to a specific user's outgoing channel non-blocking.
func (h *Hub) sendToUser(u *User, data []byte) {
	select {
	case u.OutGoingMessages <- data:
	default:
		h.logg.Warn("outgoing channel full, dropping message to user", zap.String("userFP", u.Fingerprint))
	}
}
