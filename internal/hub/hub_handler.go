package hub

import (
	"2L1nk/internal/models"
	"encoding/json"

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
			h.logg.Info("target room not found", zap.String("roomId", payload.RoomID))
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
	}
}

func (h *Hub) handleRegisterRoom(req CreateRoomRequest) {
	roomHost := h.getUser(req.Host.PublicKeyFingerprint)
	if roomHost == nil {
		h.logg.Debug("host user not found", zap.String("fingerprint", req.Host.PublicKeyFingerprint))
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

}
func (h *Hub) handleUnregisterRoom(roomID string) {}

func (h *Hub) handleRegisterUser(user *User) {
	h.Users[user.Fingerprint] = user
	h.logg.Info("user connected", zap.String("username", user.Username), zap.String("fingerprint", user.Fingerprint))
}
func (h *Hub) handleUnregisterUser(user *User) {
	delete(h.Users, user.Fingerprint)
	h.logg.Info("user disconnected", zap.String("username", user.Username), zap.String("fingerprint", user.Fingerprint))

}

func (h *Hub) handleJoinRoom(req RoomMembersChangeRequest)  {}
func (h *Hub) handleLeaveRoom(req RoomMembersChangeRequest) {}

func (h *Hub) handleBroadcast(req BroadcastRequest) {}
