package service

import (
	"2L1nk/internal/gate"
	infradb "2L1nk/internal/infrastructure/db"
	"2L1nk/internal/logger"
	"2L1nk/internal/models"
	"2L1nk/internal/session"
	"2L1nk/internal/utils"
	"crypto/ed25519"
	"encoding/base64"
	"time"

	"github.com/google/uuid"
)

type UserRepository interface {
	GetByFingerprint(fingerprint string) (*infradb.UserRecord, error)
	GetAllUsers() ([]infradb.UserRecord, error)
	Create(u *infradb.UserRecord) error
	UpdateUsername(fingerprint, username string) error
	UsernameExists(username string) (bool, error)
}

type GateService struct {
	gate     *gate.Gate
	store    *session.Store
	userRepo UserRepository
	log      *logger.Logger
}

func NewGateService(g *gate.Gate, store *session.Store, userRepo UserRepository, log *logger.Logger) *GateService {
	return &GateService{
		gate:     g,
		store:    store,
		userRepo: userRepo,
		log:      log,
	}
}

type GateRequest struct {
	GateToken       string
	PublicKey       ed25519.PublicKey
	X25519PublicKey []byte
	Username        string
	Mode            models.UserMode
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

	fp := utils.FingerprintEd25519(req.PublicKey)

	existing, err := s.userRepo.GetByFingerprint(fp)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		taken, err := s.userRepo.UsernameExists(req.Username)
		if err != nil {
			return nil, err
		}
		if taken {
			return nil, ErrUsernameTaken
		}
		if err := s.userRepo.Create(&infradb.UserRecord{
			Fingerprint:     fp,
			PublicKey:       base64.StdEncoding.EncodeToString(req.PublicKey),
			X25519PublicKey: base64.StdEncoding.EncodeToString(req.X25519PublicKey),
			Username:        req.Username,
			Mode:            int(req.Mode),
			CreatedAt:       time.Now().Unix(),
		}); err != nil {
			return nil, err
		}
	} else {
		if req.Username != existing.Username {
			taken, err := s.userRepo.UsernameExists(req.Username)
			if err != nil {
				return nil, err
			}
			if taken {
				return nil, ErrUsernameTaken
			}
			if err := s.userRepo.UpdateUsername(fp, req.Username); err != nil {
				return nil, err
			}
		}
	}

	sessionID := uuid.New().String()

	s.store.Add(&session.User{
		SessionID:            sessionID,
		PublicKey:            req.PublicKey,
		X25519PublicKey:      req.X25519PublicKey,
		PublicKeyFingerprint: fp,
		Username:             req.Username,
		Mode:                 req.Mode,
	})

	return &GateResult{SessionID: sessionID}, nil
}

// GetUserByFingerprint returns the DB record for a persistent user.
func (s *GateService) GetUserByFingerprint(fingerprint string) (*infradb.UserRecord, error) {
	return s.userRepo.GetByFingerprint(fingerprint)
}

// GetAllUsers returns all persistent users from the DB.
func (s *GateService) GetAllUsers() ([]infradb.UserRecord, error) {
	return s.userRepo.GetAllUsers()
}
