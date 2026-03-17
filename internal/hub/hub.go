package hub

import (
	"2L1nk/internal/logger"
	"2L1nk/internal/session"
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
	OwnerFP string
	RoomID  string
	UserFP  string
}

type CreateRoomRequest struct {
	Host         *session.User
	GroupName    string
	ResponseChan chan string
}

type Room struct {
	Name   string
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
			h.handleRegisterRoom(req)

		case newUser := <-h.RegisterUser:
			h.handleRegisterUser(newUser)

		case user := <-h.UnregisterUser:
			h.handleUnregisterUser(user)

		case msg := <-h.InboundMessages:
			h.handleInboundMessage(msg)

		case req := <-h.JoinRoom:
			h.handleJoinRoom(req)
		}
	}
}
