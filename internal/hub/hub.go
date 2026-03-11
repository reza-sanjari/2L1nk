package hub

import (
	"2L1nk/internal/models"
	"2L1nk/internal/session"
	"encoding/json"
	"fmt"
	"os/user"
)

type Hub struct {
	s               *session.Store
	Rooms           map[string]*room
	Users           map[string]*User
	Broadcast       chan WSMessageEnvelope
	InboundMessages chan WSMessageEnvelope
	RegisterRoom    chan CreateRoomRequest
	UnregisterRoom  chan map[string]*room
	RegisterUser    chan *User
	UnregisterUser  chan *User
	JoinRoom        chan JoinRoomRequest
	LeaveRoom       chan LeaveRoomRequest
}

type JoinRoomRequest struct {
	RoomID string
	User   *user.User
}

type LeaveRoomRequest struct {
	RoomID string
	User   *user.User
}

type WSMessageEnvelope struct {
	Type    models.WSEventType `json:"type"`
	Payload json.RawMessage
}

type room struct {
	roomID string
	users  map[string]*User
	epoch  int64
}

type CreateRoomRequest struct {
	Host         string
	ResponseChan chan string
}

func New(s *session.Store) *Hub {
	return &Hub{
		s:               s,
		Rooms:           make(map[string]*room),
		Users:           make(map[string]*User),
		Broadcast:       make(chan WSMessageEnvelope),
		InboundMessages: make(chan WSMessageEnvelope),
		RegisterRoom:    make(chan CreateRoomRequest),
		UnregisterRoom:  make(chan map[string]*room),
		RegisterUser:    make(chan *User),
		UnregisterUser:  make(chan *User),
		JoinRoom:        make(chan JoinRoomRequest),
		LeaveRoom:       make(chan LeaveRoomRequest)}
}

func (h *Hub) Run() {
	for {
		select {
		case newRoom := <-h.RegisterRoom:
			fmt.Printf("register room %v\n", newRoom)

		case newUser := <-h.RegisterUser:
			fmt.Printf("register username %v\n", newUser)
		}
	}
}
func (h *Hub) Status() (string, error) {
	return "OK", nil
}
