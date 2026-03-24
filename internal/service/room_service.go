package service

import (
	"2L1nk/internal/hub"
	infradb "2L1nk/internal/infrastructure/db"
	"2L1nk/internal/logger"
	"2L1nk/internal/models"
	"sort"
)

type RoomRepository interface {
	Create(room *infradb.RoomRecord) error
	GetByID(roomID string) (*infradb.RoomRecord, error)
	GetRoomsByMember(fp string) ([]*infradb.RoomRecord, error)
	AddMember(roomID, memberFP string, joinedAt int64) error
	GetMembersOfRoom(roomID string) ([]string, error)
	GetMembersWithPublicKeys(roomID string) ([]infradb.MemberKeyInfo, error)
	GetRoomMembersWithDetails(roomID string) ([]infradb.MemberDetailInfo, error)
	RemoveMember(roomID, memberFP string) error
	Delete(roomID string) error
	UpdateHostFP(roomID, newHostFP string) error
	UpdateEpochAndKeyCreator(roomID string, epoch int64, keyCreatorFP string) error
	StoreKeySlots(slots []infradb.KeySlotRecord) error
	GetKeySlotsByRecipient(recipientFP string) ([]infradb.KeySlotRecord, error)
	DeleteKeySlots(roomID string) error
	HasKeySlots(roomID string, epoch int64) (bool, error)
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
		HostFP:       p.CreatorFP,
		KeyCreatorFP: p.CreatorFP,
		CreatedAt:    p.CreatedAt,
	}); err != nil {
		return err
	}
	return s.repo.AddMember(p.RoomID, p.CreatorFP, p.CreatedAt)
}

// AddMemberDirect persists a member join to room_members directly (DB-first flow).
// Skips silently if the room is not in the DB (ephemeral room).
func (s *RoomService) AddMemberDirect(roomID, memberFP string, joinedAt int64) error {
	room, err := s.repo.GetByID(roomID)
	if err != nil {
		return err
	}
	if room == nil {
		return nil // ephemeral room, nothing to persist
	}
	return s.repo.AddMember(roomID, memberFP, joinedAt)
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

func (s *RoomService) UpdateHostFP(roomID, newHostFP string) error {
	return s.repo.UpdateHostFP(roomID, newHostFP)
}

func (s *RoomService) UpdateEpochAndKeyCreator(roomID string, epoch int64, keyCreatorFP string) error {
	return s.repo.UpdateEpochAndKeyCreator(roomID, epoch, keyCreatorFP)
}

func (s *RoomService) StoreKeySlots(slots []infradb.KeySlotRecord) error {
	return s.repo.StoreKeySlots(slots)
}

func (s *RoomService) GetKeySlotsByRecipient(fp string) ([]infradb.KeySlotRecord, error) {
	return s.repo.GetKeySlotsByRecipient(fp)
}

func (s *RoomService) GetMembersWithPublicKeys(roomID string) ([]infradb.MemberKeyInfo, error) {
	return s.repo.GetMembersWithPublicKeys(roomID)
}

func (s *RoomService) GetRoomMembersWithDetails(roomID string) ([]infradb.MemberDetailInfo, error) {
	return s.repo.GetRoomMembersWithDetails(roomID)
}

func (s *RoomService) DeleteKeySlotsByRoom(roomID string) error {
	return s.repo.DeleteKeySlots(roomID)
}

func (s *RoomService) HasKeySlots(roomID string, epoch int64) (bool, error) {
	return s.repo.HasKeySlots(roomID, epoch)
}

// MemberWithMode holds a member fingerprint and their mode for lex selection.
type MemberWithMode struct {
	FP   string
	Mode models.UserMode
}

// SelectNextByLex selects the next host or key creator from a list of members.
// Priority: online persistent lex lowest → online ephemeral lex lowest →
// offline persistent lex lowest → offline ephemeral lex lowest.
// excludeFP is skipped (the member being removed or transferred away from).
func SelectNextByLex(members []MemberWithMode, onlineSet map[string]bool, excludeFP string) string {
	var onlinePersistent, onlineEphemeral, offlinePersistent, offlineEphemeral []string
	for _, m := range members {
		if m.FP == excludeFP {
			continue
		}
		if onlineSet[m.FP] {
			if m.Mode == models.UserModePersistent {
				onlinePersistent = append(onlinePersistent, m.FP)
			} else {
				onlineEphemeral = append(onlineEphemeral, m.FP)
			}
		} else {
			if m.Mode == models.UserModePersistent {
				offlinePersistent = append(offlinePersistent, m.FP)
			} else {
				offlineEphemeral = append(offlineEphemeral, m.FP)
			}
		}
	}
	for _, bucket := range [][]string{onlinePersistent, onlineEphemeral, offlinePersistent, offlineEphemeral} {
		if len(bucket) > 0 {
			sort.Strings(bucket)
			return bucket[0]
		}
	}
	return ""
}
