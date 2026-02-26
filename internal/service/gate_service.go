package service

import (
	"2L1nk/internal/gate"
)

type GateService struct {
	gate *gate.Gate
}

func NewGateService(g *gate.Gate) *GateService {
	return &GateService{gate: g}
}

// Authorize validates a gate token.
// Returns the authorized status and any error.
func (s *GateService) Authorize(token string) (bool, error) {
	return s.gate.Validate(token)
}
