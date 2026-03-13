package hub

import (
	"2L1nk/internal/logger"
	"2L1nk/internal/models"
	"2L1nk/internal/session"
	"encoding/json"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

type Hub struct {
	logg            *logger.Logger
	s               *session.Store
	Rooms           map[string]*Room
	Users           map[string]*User
	Broadcast       chan BroadcastRequest
	InboundMessages chan WSMessageEnvelope
	RegisterRoom    chan CreateRoomRequest
	UnregisterRoom  chan string
	RegisterUser    chan *User
	UnregisterUser  chan *User
	JoinRoom        chan RoomMembersChangeRequest
	LeaveRoom       chan RoomMembersChangeRequest
}

type RoomMembersChangeRequest struct {
	RoomID string
	User   *User
}

type CreateRoomRequest struct {
	Host         *session.User
	GroupName    string
	ResponseChan chan string
}

type Room struct {
	name   string
	RoomID string
	Host   *User
	Users  map[string]*User
	Epoch  int64
}

func New(s *session.Store, logg *logger.Logger) *Hub {
	return &Hub{
		logg:            logg,
		s:               s,
		Rooms:           make(map[string]*Room),
		Users:           make(map[string]*User),
		Broadcast:       make(chan BroadcastRequest),
		InboundMessages: make(chan WSMessageEnvelope),
		RegisterRoom:    make(chan CreateRoomRequest),
		UnregisterRoom:  make(chan string),
		RegisterUser:    make(chan *User),
		UnregisterUser:  make(chan *User),
		JoinRoom:        make(chan RoomMembersChangeRequest),
		LeaveRoom:       make(chan RoomMembersChangeRequest)}
}

func (h *Hub) Run() {
	for {
		select {
		case req := <-h.RegisterRoom:
			roomHost := h.getUser(req.Host.PublicKeyFingerprint)
			if roomHost == nil {
				h.logg.Debug("host user not found", zap.String("fingerprint", req.Host.PublicKeyFingerprint))
				req.ResponseChan <- ""
				continue
			}
			roomID := uuid.NewString()
			h.Rooms[roomID] = &Room{
				name:   req.GroupName,
				RoomID: roomID,
				Host:   roomHost,
				Users:  map[string]*User{roomHost.Fingerprint: roomHost},
				Epoch:  0,
			}

			req.ResponseChan <- roomID
			h.logg.Debug("room created", zap.String("roomID", roomID), zap.String("host", req.Host.Username))

		case newUser := <-h.RegisterUser:
			h.Users[newUser.Fingerprint] = newUser
			h.logg.Info("user connected", zap.String("username", newUser.Username), zap.String("fingerprint", newUser.Fingerprint))

		case user := <-h.UnregisterUser:
			delete(h.Users, user.Fingerprint)
			h.logg.Info("user disconnected", zap.String("username", user.Username), zap.String("fingerprint", user.Fingerprint))

		case msg := <-h.InboundMessages:
			switch msg.Type {
			case models.Message:
				var payload MessagePayload
				err := json.Unmarshal(msg.Payload, &payload)
				h.logg.Debug("message received", zap.String("user", msg.Sender.Username))
				if err != nil {
					h.logg.Error("Failed to unmarshal payload", zap.String("payload", string(msg.Payload)), zap.Error(err))
					continue
				}
				h.logg.Info("Received message", zap.String("user", string(msg.Sender.Username)), zap.String("text", payload.Ciphertext))
				targetRoom := h.getRoom(payload.RoomID)
				h.logg.Debug("target room found")
				if targetRoom == nil {
					h.logg.Info("target room not found", zap.String("roomId", payload.RoomID))
					continue
				}
				if !h.isUserInRoom(msg.Sender, targetRoom) {
					h.logg.Debug("message not sent", zap.String("error", "user not in room"), zap.String("fingerprint", msg.Sender.Username))
					continue
				}
				h.logg.Debug("user is in room")
				data, err := json.Marshal(msg)
				if err != nil {
					h.logg.Error("failed to marshal message", zap.Error(err))
					continue
				}
				for _, user := range targetRoom.Users {
					user.OutGoingMessages <- data
				}
			}

		}
	}
}
func (h *Hub) Status() (string, error) {
	return "OK", nil
}

func (h *Hub) getUser(fingerPrint string) *User {
	user, _ := h.Users[fingerPrint]
	return user
}

func (h *Hub) isUserInRoom(user *User, room *Room) bool {
	_, ok := room.Users[user.Fingerprint]
	return ok
}

func (h *Hub) getRoom(roomId string) *Room {
	return h.Rooms[roomId]
}
