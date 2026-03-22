package hub

import (
	"2L1nk/internal/models"
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

func (h *Hub) handleInboundMessage(msg WSMessageEnvelope) {
	switch msg.Type {
	case models.Message:
		var payload MessagePayload
		err := json.Unmarshal(msg.Payload, &payload)
		h.logg.Debug("message received", zap.String("user", msg.Sender.Username))
		if err != nil {
			h.logg.Error("Failed to unmarshal payload", zap.String("payload", string(msg.Payload)), zap.Error(err))
			return
		}
		targetRoom := h.getRoom(payload.RoomID)
		if targetRoom == nil {
			h.logg.Debug("target room not found", zap.String("roomId", payload.RoomID))
			return
		}
		if !h.isUserInRoom(msg.Sender, targetRoom) {
			h.logg.Debug("message not sent", zap.String("error", "user not in room"), zap.String("fingerprint", msg.Sender.Username))
			return
		}
		data, err := json.Marshal(outboundEnvelope{
			SenderFP: msg.Sender.Fingerprint,
			Type:     msg.Type,
			Payload:  msg.Payload,
		})
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
		Name:   req.GroupName,
		RoomID: roomID,
		Host:   roomHost,
		Users:  map[string]*User{roomHost.Fingerprint: roomHost},
		Epoch:  0,
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
	// Update pointer in all rooms this user is a member of (reconnect after page reload)
	for _, room := range h.Rooms {
		if _, ok := room.Users[user.Fingerprint]; ok {
			room.Users[user.Fingerprint] = user
		}
	}
	h.logg.Info("user connected", zap.String("username", user.Username), zap.String("fingerprint", user.Fingerprint))
}

func (h *Hub) handleUnregisterUser(user *User) {
	delete(h.Users, user.Fingerprint)
	h.logg.Info("user disconnected", zap.String("username", user.Username), zap.String("fingerprint", user.Fingerprint))
}

func (h *Hub) handleJoinRoom(req RoomMembersChangeRequest) {
	ownerUser := h.getUser(req.OwnerFP)
	room := h.getRoom(req.RoomID)
	if ownerUser == nil || room == nil {
		h.logg.Debug("join room failed: owner or room not found", zap.String("ownerFP", req.OwnerFP), zap.String("roomID", req.RoomID))
		return
	}
	if h.isUserInRoom(ownerUser, room) {
		newUser := h.getUser(req.UserFP)
		if newUser == nil {
			h.logg.Debug("user not online can't add to room", zap.String("fingerprint", req.UserFP))
		} else {
			room.Users[newUser.Fingerprint] = newUser
			h.logg.Debug("user joined", zap.String("fingerprint", req.UserFP))

			h.emit(HubEvent{
				Type: HubEventMemberJoined,
				Payload: MemberJoinedPayload{
					RoomID:     req.RoomID,
					MemberFP:   newUser.Fingerprint,
					MemberMode: newUser.Mode,
					JoinedAt:   time.Now().Unix(),
				},
			})
		}
	}
}

func (h *Hub) handleLeaveRoom(req RoomMembersChangeRequest) {
	room := h.getRoom(req.RoomID)
	if room == nil {
		h.logg.Debug("leave room failed: room not found", zap.String("roomID", req.RoomID))
		return
	}
	if room.Host.Fingerprint != req.OwnerFP {
		h.logg.Debug("leave room failed: not host", zap.String("ownerFP", req.OwnerFP))
		return
	}
	delete(room.Users, req.UserFP)
	h.logg.Debug("user removed from room", zap.String("userFP", req.UserFP), zap.String("roomID", req.RoomID))
}

func (h *Hub) handleBroadcast(req BroadcastRequest) {}
