package service

import (
	"2L1nk/internal/logger"
	"time"
)

type HealthRepository interface {
	Ping() error
}

type HealthService struct {
	repo   HealthRepository
	logger *logger.Logger
}

func NewHealthService(repo HealthRepository, log *logger.Logger) *HealthService {
	return &HealthService{
		repo:   repo,
		logger: log,
	}
}

func (s *HealthService) GetStatus() (map[string]any, error) {
	if err := s.repo.Ping(); err != nil {
		return nil, err
	}

	return map[string]any{
		"status":    "ok",
		"timestamp": time.Now().UTC(),
		"mode":      "memory",
	}, nil
}
