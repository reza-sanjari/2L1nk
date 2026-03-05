package hub

import (
	"2L1nk/internal/models"
	"2L1nk/internal/session"
	"encoding/json"
)

type Hub struct{ s *session.Store }

type WSMessageEnvelope struct {
	Type    models.WSEventType `json:"type"`
	payload json.RawMessage
}

type room struct {
	roomID string
	users  map[string]*User
	epoch  int64
}

func New(s *session.Store) *Hub {
	return &Hub{s: s}
}

func (h *Hub) Status() (string, error) {
	return "OK", nil
}
