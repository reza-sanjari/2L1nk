package service

import "time"

type HealthRepository interface {
	Ping() error
}

type HealthService struct {
	repo HealthRepository
}

func NewHealthService(repo HealthRepository) *HealthService {
	return &HealthService{repo: repo}
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
