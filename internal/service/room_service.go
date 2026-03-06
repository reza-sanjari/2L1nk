package service

import (
	"2L1nk/internal/logger"
)

type RoomRepository interface {
	AddRoomToDb() error
}

type RoomService struct {
	repo   RoomRepository
	logger *logger.Logger
}

func NewRoomService(repo RoomRepository, log *logger.Logger) *RoomService {
	return &RoomService{
		repo:   repo,
		logger: log,
	}
}

func (r *RoomService) AddRoomToDb() error {
	return r.repo.AddRoomToDb()
}
