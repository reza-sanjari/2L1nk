package service

import (
	"2L1nk/internal/hub"
	infradb "2L1nk/internal/infrastructure/db"
	"2L1nk/internal/logger"
	"2L1nk/internal/models"
)

type MessageRepository interface {
	Save(msg *infradb.MessageRecord) error
	GetByRoom(roomID string, limit, offset int) ([]*infradb.MessageRecord, error)
	DeleteByRoom(roomID string) error
	DeleteBySenderFP(senderFP string) (int64, error)
}

type MessageService struct {
	msgRepo MessageRepository
	log     *logger.Logger
}

func NewMessageService(msgRepo MessageRepository, log *logger.Logger) *MessageService {
	return &MessageService{msgRepo: msgRepo, log: log}
}

// ProcessMessage saves a message to the DB unless the sender is ephemeral.
func (s *MessageService) ProcessMessage(p hub.MessageCreatedPayload) error {
	if p.SenderMode == models.UserModeEphemeral {
		return nil
	}
	return s.msgRepo.Save(&infradb.MessageRecord{
		ID:           p.ID,
		RoomID:       p.RoomID,
		SenderFP:     p.SenderFP,
		Epoch:        p.Epoch,
		Type:         0,
		Ciphertext:   p.Ciphertext,
		Signature:    p.Signature,
		SigTimestamp: p.SigTimestamp,
		SigNonce:     p.SigNonce,
		CreatedAt:    p.CreatedAt,
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
