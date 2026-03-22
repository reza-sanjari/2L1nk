package service

import (
	"2L1nk/internal/hub"
	infradb "2L1nk/internal/infrastructure/db"
	"2L1nk/internal/logger"
	"2L1nk/internal/models"
)

type RoomRepository interface {
	Create(room *infradb.RoomRecord) error
	GetByID(roomID string) (*infradb.RoomRecord, error)
	GetRoomsByMember(fp string) ([]*infradb.RoomRecord, error)
	AddMember(roomID, memberFP string, joinedAt int64) error
	GetMembersOfRoom(roomID string) ([]string, error)
	RemoveMember(roomID, memberFP string) error
	Delete(roomID string) error
	UpdateHost(roomID, newHostFP string) error
}

type RoomService struct {
	repo RoomRepository
	log  *logger.Logger
}

func NewRoomService(repo RoomRepository, log *logger.Logger) *RoomService {
	return &RoomService{repo: repo, log: log}
}

// CreateRoom persists a room to the DB only when the creator is a persistent user.
// Also inserts the creator into room_members.
func (s *RoomService) CreateRoom(p hub.RoomCreatedPayload) error {
	if p.CreatorMode != models.UserModePersistent {
		return nil
	}
	if err := s.repo.Create(&infradb.RoomRecord{
		ID:           p.RoomID,
		Name:         p.Name,
		CurrentEpoch: 0,
		KeyCreatorFP: p.CreatorFP,
		CreatedAt:    p.CreatedAt,
	}); err != nil {
		return err
	}
	return s.repo.AddMember(p.RoomID, p.CreatorFP, p.CreatedAt)
}

// AddMember persists a member join to room_members.
// Skips silently if the room is not in the DB (ephemeral room).
// The repo's conditional INSERT skips ephemeral members automatically.
func (s *RoomService) AddMember(p hub.MemberJoinedPayload) error {
	room, err := s.repo.GetByID(p.RoomID)
	if err != nil {
		return err
	}
	if room == nil {
		return nil // ephemeral room, nothing to persist
	}
	return s.repo.AddMember(p.RoomID, p.MemberFP, p.JoinedAt)
}

// GetUserRooms returns all rooms a persistent user belongs to from the DB.
func (s *RoomService) GetUserRooms(fp string) ([]*infradb.RoomRecord, error) {
	return s.repo.GetRoomsByMember(fp)
}

func (s *RoomService) GetRoomByID(roomID string) (*infradb.RoomRecord, error) {
	return s.repo.GetByID(roomID)
}

func (s *RoomService) GetRoomMembers(roomID string) ([]string, error) {
	return s.repo.GetMembersOfRoom(roomID)
}

func (s *RoomService) RemoveMember(roomID, memberFP string) error {
	return s.repo.RemoveMember(roomID, memberFP)
}

func (s *RoomService) DeleteRoom(roomID string) error {
	return s.repo.Delete(roomID)
}

func (s *RoomService) UpdateHost(roomID, newHostFP string) error {
	return s.repo.UpdateHost(roomID, newHostFP)
}
