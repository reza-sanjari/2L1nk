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
	Broadcast       chan WSMessageEnvelope
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
		Broadcast:       make(chan WSMessageEnvelope),
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

		case newUser := <-h.RegisterUser:
			h.Users[newUser.Fingerprint] = newUser
			h.logg.Info("user connected", zap.String("username", newUser.Username), zap.String("fingerprint", newUser.Fingerprint))

		case user := <-h.UnregisterUser:
			delete(h.Users, user.Fingerprint)

		case msg := <-h.InboundMessages:
			switch msg.Type {
			case models.Message:
				var p MessagePayload
				err := json.Unmarshal(msg.Payload, &p)
				if err != nil {
					h.logg.Error("Failed to unmarshal payload", zap.String("payload", string(msg.Payload)), zap.Error(err))
					return
				}
				h.logg.Debug(
					"received message",
					zap.String("message", p.Ciphertext),
					zap.String("sender", msg.Sender.Username),
				)
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
