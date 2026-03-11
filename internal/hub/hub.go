package hub

import (
	"2L1nk/internal/logger"
	"2L1nk/internal/models"
	"2L1nk/internal/session"
	"encoding/json"
	"fmt"
)

type Hub struct {
	logg            *logger.Logger
	s               *session.Store
	Rooms           map[string]*room
	Users           map[string]*User
	Broadcast       chan WSMessageEnvelope
	InboundMessages chan WSMessageEnvelope
	RegisterRoom    chan CreateRoomRequest
	UnregisterRoom  chan map[string]*room
	RegisterUser    chan *User
	UnregisterUser  chan *User
	JoinRoom        chan RoomRequest
	LeaveRoom       chan RoomRequest
}

type RoomRequest struct {
	RoomID string
	User   *User
}

type WSMessageEnvelope struct {
	Type    models.WSEventType `json:"type"`
	Payload json.RawMessage    `json:"payload"`
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

func New(s *session.Store, logg *logger.Logger) *Hub {
	return &Hub{
		logg:            logg,
		s:               s,
		Rooms:           make(map[string]*room),
		Users:           make(map[string]*User),
		Broadcast:       make(chan WSMessageEnvelope),
		InboundMessages: make(chan WSMessageEnvelope),
		RegisterRoom:    make(chan CreateRoomRequest),
		UnregisterRoom:  make(chan map[string]*room),
		RegisterUser:    make(chan *User),
		UnregisterUser:  make(chan *User),
		JoinRoom:        make(chan RoomRequest),
		LeaveRoom:       make(chan RoomRequest)}
}

func (h *Hub) Run() {
	for {
		select {
		case newRoom := <-h.RegisterRoom:
			fmt.Printf("register room %v\n", newRoom)

		case newUser := <-h.RegisterUser:
			fmt.Printf("register username %v\n", newUser)

		case msg := <-h.InboundMessages:
			switch msg.Type {
			case models.Message:
				fmt.Printf("broadcast message %v\n", msg)

			}
		}
	}
}
func (h *Hub) Status() (string, error) {
	return "OK", nil
}
