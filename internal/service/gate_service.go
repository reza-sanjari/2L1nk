package service

import (
	"2L1nk/internal/gate"
	"2L1nk/internal/session"
	"2L1nk/internal/utils"
	"crypto/ed25519"

	"github.com/google/uuid"
)

type GateService struct {
	gate  *gate.Gate
	store *session.Store
}

func NewGateService(g *gate.Gate, store *session.Store) *GateService {
	return &GateService{
		gate:  g,
		store: store,
	}
}

type GateRequest struct {
	GateToken string
	PublicKey ed25519.PublicKey
	Username  string
	Mode      session.UserMode
}

type GateResult struct {
	SessionID string
}

func (s *GateService) Authorize(req GateRequest) (*GateResult, error) {
	validated, err := s.gate.Validate(req.GateToken)
	if err != nil {
		return nil, err
	}
	if !validated {
		return nil, ErrInvalidGateKey
	}

	if s.store.UsernameExists(req.Username) {
		return nil, ErrUsernameTaken
	}

	sessionID := uuid.New().String()

	s.store.Add(&session.User{
		SessionID:            sessionID,
		PublicKey:            req.PublicKey,
		PublicKeyFingerprint: utils.FingerprintEd25519(req.PublicKey),
		Username:             req.Username,
		Mode:                 req.Mode,
	})

	return &GateResult{SessionID: sessionID}, nil
}
