package service

import (
	"2L1nk/internal/hub"
	infradb "2L1nk/internal/infrastructure/db"
	"2L1nk/internal/logger"
)

type MessageRepository interface {
	Save(msg *infradb.MessageRecord) error
	GetByRoom(roomID string, limit, offset int) ([]*infradb.MessageRecord, error)
	DeleteByRoom(roomID string) error
	DeleteBySenderFP(senderFP string) (int64, error)
}

type MessageService struct {
	msgRepo  MessageRepository
	roomRepo RoomRepository // reuses the interface defined in room_service.go
	log      *logger.Logger
}

func NewMessageService(msgRepo MessageRepository, roomRepo RoomRepository, log *logger.Logger) *MessageService {
	return &MessageService{msgRepo: msgRepo, roomRepo: roomRepo, log: log}
}

// ProcessMessage saves a message to the DB if the room is persisted.
// If the room is not in the DB (ephemeral room), the save is skipped silently.
func (s *MessageService) ProcessMessage(p hub.MessageCreatedPayload) error {
	room, err := s.roomRepo.GetByID(p.RoomID)
	if err != nil {
		return err
	}
	if room == nil {
		return nil // ephemeral room, skip
	}
	return s.msgRepo.Save(&infradb.MessageRecord{
		ID:         p.ID,
		RoomID:     p.RoomID,
		SenderFP:   p.SenderFP,
		Epoch:      p.Epoch,
		Type:       0,
		Ciphertext: p.Ciphertext,
		CreatedAt:  p.CreatedAt,
	})
}

func (s *MessageService) GetRoomMessages(roomID string, limit, offset int) ([]*infradb.MessageRecord, error) {
	return s.msgRepo.GetByRoom(roomID, limit, offset)
}

func (s *MessageService) DeleteByRoom(roomID string) error {
	return s.msgRepo.DeleteByRoom(roomID)
}

func (s *MessageService) PurgeUserMessages(senderFP string) (int64, error) {
	return s.msgRepo.DeleteBySenderFP(senderFP)
}
