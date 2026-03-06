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
	Broadcast       chan WSMessageEnvelope
	InboundMessages chan WSMessageEnvelope
	RegisterRoom    chan CreateRoomRequest
	UnregisterRoom  chan map[string]*room
	RegisterUser    chan map[string]*user.User
	UnregisterUser  chan *user.User
}

type WSMessageEnvelope struct {
	Type    models.WSEventType `json:"type"`
	payload json.RawMessage
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
	return &Hub{s: s}
}

func (h *Hub) Run() {
	for {
		select {
		case groupOwner := <-h.RegisterRoom:
			fmt.Printf("register room %v\n", groupOwner)
		}
	}
}
func (h *Hub) Status() (string, error) {
	return "OK", nil
}
