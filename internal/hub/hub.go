package hub

import "2L1nk/internal/session"

type Hub struct{ s *session.Store }

func New(s *session.Store) *Hub {
	return &Hub{s: s}
}

func (h *Hub) Status() (string, error) {
	return "OK", nil
}
