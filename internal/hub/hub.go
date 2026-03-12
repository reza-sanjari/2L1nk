package hub

import (
	"2L1nk/internal/logger"
	"2L1nk/internal/models"
	"2L1nk/internal/session"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

type Hub struct {
	logg            *logger.Logger
	s               *session.Store
	Rooms           map[string]*room
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
	Host         string
	ResponseChan chan string
}

type WSMessageEnvelope struct {
	Sender  *User              `json:"-"` // server-only
	Type    models.WSEventType `json:"type"`
	Payload json.RawMessage    `json:"payload"`
}

type room struct {
	roomID string
	Host   string
	users  map[string]*User
	epoch  int64
}

func New(s *session.Store, logg *logger.Logger) *Hub {
	return &Hub{
		logg:            logg,
		s:               s,
		Rooms:           make(map[string]*room),
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
			roomID := uuid.NewString()

			h.Rooms[roomID] = &room{
				roomID: roomID,
				Host:   req.Host,
				users:  make(map[string]*User),
				epoch:  0,
			}

			req.ResponseChan <- roomID

		case newUser := <-h.RegisterUser:
			h.Users[newUser.Fingerprint] = newUser
			fmt.Printf("register user %v with fingerprint %v \n", newUser.Username, newUser.Fingerprint)

		case user := <-h.UnregisterUser:
			delete(h.Users, user.Fingerprint)

		case msg := <-h.InboundMessages:
			switch msg.Type {
			case models.Message:
				fmt.Printf("message received from %v\n", msg.Sender.Username)
			}
		}
	}
}
func (h *Hub) Status() (string, error) {
	return "OK", nil
}
