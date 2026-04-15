package db

import (
	"database/sql"
	"fmt"
)

type RoomRecord struct {
	ID           string
	Name         string
	CurrentEpoch int64
	HostFP       string // empty string when NULL in DB
	KeyCreatorFP string // empty string when NULL in DB
	CreatedAt    int64
}

type RoomRepository struct {
	db *sql.DB
}

func NewRoomRepository(db *sql.DB) *RoomRepository {
	return &RoomRepository{db: db}
}

func (r *RoomRepository) Create(room *RoomRecord) error {
	var hostFP, keyCreatorFP interface{}
	if room.HostFP != "" {
		hostFP = room.HostFP
	}
	if room.KeyCreatorFP != "" {
		keyCreatorFP = room.KeyCreatorFP
	}
	_, err := r.db.Exec(
		`INSERT INTO rooms (id, name, current_epoch, host_fp, key_creator_fp, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		room.ID, room.Name, room.CurrentEpoch, hostFP, keyCreatorFP, room.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create room: %w", err)
	}
	return nil
}

// GetByID returns the room with the given ID, or nil if not found.
func (r *RoomRepository) GetByID(roomID string) (*RoomRecord, error) {
	row := r.db.QueryRow(
		`SELECT id, name, current_epoch, host_fp, key_creator_fp, created_at FROM rooms WHERE id = ?`,
		roomID,
	)
	rec := &RoomRecord{}
	var hostFP, keyCreatorFP sql.NullString
	err := row.Scan(&rec.ID, &rec.Name, &rec.CurrentEpoch, &hostFP, &keyCreatorFP, &rec.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get room by id: %w", err)
	}
	rec.HostFP = hostFP.String
	rec.KeyCreatorFP = keyCreatorFP.String
	return rec, nil
}

// GetRoomsByMember returns all rooms a user is a member of.
func (r *RoomRepository) GetRoomsByMember(fp string) ([]*RoomRecord, error) {
	rows, err := r.db.Query(
		`SELECT r.id, r.name, r.current_epoch, r.host_fp, r.key_creator_fp, r.created_at
		 FROM rooms r
		 JOIN room_members rm ON r.id = rm.room_id
		 WHERE rm.member_fp = ?`,
		fp,
	)
	if err != nil {
		return nil, fmt.Errorf("get rooms by member: %w", err)
	}
	defer rows.Close()

	var rooms []*RoomRecord
	for rows.Next() {
		rec := &RoomRecord{}
		var hostFP, keyCreatorFP sql.NullString
		if err := rows.Scan(&rec.ID, &rec.Name, &rec.CurrentEpoch, &hostFP, &keyCreatorFP, &rec.CreatedAt); err != nil {
			return nil, err
		}
		rec.HostFP = hostFP.String
		rec.KeyCreatorFP = keyCreatorFP.String
		rooms = append(rooms, rec)
	}
	return rooms, rows.Err()
}

// GetMembersOfRoom returns the fingerprints of all members of a room.
func (r *RoomRepository) GetMembersOfRoom(roomID string) ([]string, error) {
	rows, err := r.db.Query(
		`SELECT member_fp FROM room_members WHERE room_id = ?`,
		roomID,
	)
	if err != nil {
		return nil, fmt.Errorf("get members of room: %w", err)
	}
	defer rows.Close()

	var fps []string
	for rows.Next() {
		var fp string
		if err := rows.Scan(&fp); err != nil {
			return nil, err
		}
		fps = append(fps, fp)
	}
	return fps, rows.Err()
}

// RemoveMember removes a member from room_members.
func (r *RoomRepository) RemoveMember(roomID, memberFP string) error {
	_, err := r.db.Exec(
		`DELETE FROM room_members WHERE room_id = ? AND member_fp = ?`,
		roomID, memberFP,
	)
	if err != nil {
		return fmt.Errorf("remove room member: %w", err)
	}
	return nil
}

// Delete removes a room and all its members from the DB.
func (r *RoomRepository) Delete(roomID string) error {
	if _, err := r.db.Exec(`DELETE FROM room_members WHERE room_id = ?`, roomID); err != nil {
		return fmt.Errorf("delete room members: %w", err)
	}
	if _, err := r.db.Exec(`DELETE FROM rooms WHERE id = ?`, roomID); err != nil {
		return fmt.Errorf("delete room: %w", err)
	}
	return nil
}

// UpdateHostFP sets a new host_fp for the room.
func (r *RoomRepository) UpdateHostFP(roomID, newHostFP string) error {
	_, err := r.db.Exec(
		`UPDATE rooms SET host_fp = ? WHERE id = ?`,
		newHostFP, roomID,
	)
	if err != nil {
		return fmt.Errorf("update room host_fp: %w", err)
	}
	return nil
}

// DeleteKeySlots removes all epoch key slots for a room.
func (r *RoomRepository) DeleteKeySlots(roomID string) error {
	_, err := r.db.Exec(`DELETE FROM room_key_slots WHERE room_id = ?`, roomID)
	if err != nil {
		return fmt.Errorf("delete key slots: %w", err)
	}
	return nil
}

// HasKeySlots reports whether any key slots exist for the given room and epoch.
func (r *RoomRepository) HasKeySlots(roomID string, epoch int64) (bool, error) {
	var count int
	err := r.db.QueryRow(
		`SELECT COUNT(*) FROM room_key_slots WHERE room_id = ? AND epoch = ? LIMIT 1`,
		roomID, epoch,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("has key slots: %w", err)
	}
	return count > 0, nil
}

// AddMember conditionally inserts the member into room_members.
// Silently skips if memberFP is not in the users table (ephemeral user)
// or if the membership already exists.
func (r *RoomRepository) AddMember(roomID, memberFP string, joinedAt int64) error {
	_, err := r.db.Exec(
		`INSERT OR IGNORE INTO room_members (room_id, member_fp, joined_at)
		 SELECT ?, fingerprint, ? FROM users WHERE fingerprint = ?`,
		roomID, joinedAt, memberFP,
	)
	if err != nil {
		return fmt.Errorf("add room member: %w", err)
	}
	return nil
}

// UpdateEpochAndKeyCreator sets the current epoch and key creator for a room.
func (r *RoomRepository) UpdateEpochAndKeyCreator(roomID string, epoch int64, keyCreatorFP string) error {
	var fp interface{}
	if keyCreatorFP != "" {
		fp = keyCreatorFP
	}
	_, err := r.db.Exec(
		`UPDATE rooms SET current_epoch = ?, key_creator_fp = ? WHERE id = ?`,
		epoch, fp, roomID,
	)
	if err != nil {
		return fmt.Errorf("update epoch and key creator: %w", err)
	}
	return nil
}

// KeySlotRecord represents a stored encrypted epoch key for one recipient.
type KeySlotRecord struct {
	RoomID       string
	Epoch        int64
	RecipientFP  string
	EncryptedKey []byte
	CreatedAt    int64
}

// StoreKeySlots inserts or replaces encrypted key slots for an epoch.
func (r *RoomRepository) StoreKeySlots(slots []KeySlotRecord) error {
	for _, s := range slots {
		_, err := r.db.Exec(
			`INSERT OR REPLACE INTO room_key_slots (room_id, epoch, recipient_fp, encrypted_key, created_at)
			 SELECT ?, ?, fingerprint, ?, ?
			 FROM users WHERE fingerprint = ?`,
			s.RoomID, s.Epoch, s.EncryptedKey, s.CreatedAt, s.RecipientFP,
		)
		if err != nil {
			return fmt.Errorf("store key slot for %s: %w", s.RecipientFP, err)
		}
	}
	return nil
}

// GetKeySlotsByRecipient returns all stored key slots for a user across all rooms.
// Used to re-deliver pending epoch keys when a persistent user reconnects.
func (r *RoomRepository) GetKeySlotsByRecipient(recipientFP string) ([]KeySlotRecord, error) {
	rows, err := r.db.Query(
		`SELECT room_id, epoch, encrypted_key, created_at
		 FROM room_key_slots
		 WHERE recipient_fp = ?
		 ORDER BY room_id, epoch`,
		recipientFP,
	)
	if err != nil {
		return nil, fmt.Errorf("get key slots: %w", err)
	}
	defer rows.Close()

	var slots []KeySlotRecord
	for rows.Next() {
		s := KeySlotRecord{RecipientFP: recipientFP}
		if err := rows.Scan(&s.RoomID, &s.Epoch, &s.EncryptedKey, &s.CreatedAt); err != nil {
			return nil, err
		}
		slots = append(slots, s)
	}
	return slots, rows.Err()
}

// GetKeySlotsByRoomAndRecipient returns stored key slots for a specific room and user,
// ordered by epoch descending, with limit/offset pagination.
func (r *RoomRepository) GetKeySlotsByRoomAndRecipient(roomID, recipientFP string, limit, offset int) ([]KeySlotRecord, error) {
	rows, err := r.db.Query(
		`SELECT room_id, epoch, encrypted_key, created_at
		 FROM room_key_slots
		 WHERE room_id = ? AND recipient_fp = ?
		 ORDER BY epoch DESC
		 LIMIT ? OFFSET ?`,
		roomID, recipientFP, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("get key slots by room: %w", err)
	}
	defer rows.Close()

	var slots []KeySlotRecord
	for rows.Next() {
		s := KeySlotRecord{RoomID: roomID, RecipientFP: recipientFP}
		if err := rows.Scan(&s.RoomID, &s.Epoch, &s.EncryptedKey, &s.CreatedAt); err != nil {
			return nil, err
		}
		slots = append(slots, s)
	}
	return slots, rows.Err()
}

// MemberKeyInfo holds a member fingerprint, their X25519 public key, and mode.
type MemberKeyInfo struct {
	Fingerprint     string
	X25519PublicKey string
	Mode            int
}

type MemberDetailInfo struct {
	Fingerprint     string
	Username        string
	X25519PublicKey string
	Mode            int
}

// GetMembersWithPublicKeys returns the fingerprint, X25519 public key, and mode
// for all members of a room.
func (r *RoomRepository) GetMembersWithPublicKeys(roomID string) ([]MemberKeyInfo, error) {
	rows, err := r.db.Query(
		`SELECT u.fingerprint, u.x25519_public_key, u.mode
		 FROM room_members rm
		 JOIN users u ON rm.member_fp = u.fingerprint
		 WHERE rm.room_id = ?`,
		roomID,
	)
	if err != nil {
		return nil, fmt.Errorf("get members with public keys: %w", err)
	}
	defer rows.Close()

	var members []MemberKeyInfo
	for rows.Next() {
		var m MemberKeyInfo
		if err := rows.Scan(&m.Fingerprint, &m.X25519PublicKey, &m.Mode); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}

// GetRoomMembersWithDetails returns fingerprint, username, X25519 public key, and mode
// for all members of a room.
func (r *RoomRepository) GetRoomMembersWithDetails(roomID string) ([]MemberDetailInfo, error) {
	rows, err := r.db.Query(
		`SELECT u.fingerprint, u.username, u.x25519_public_key, u.mode
		 FROM room_members rm
		 JOIN users u ON rm.member_fp = u.fingerprint
		 WHERE rm.room_id = ?`,
		roomID,
	)
	if err != nil {
		return nil, fmt.Errorf("get room members with details: %w", err)
	}
	defer rows.Close()

	var members []MemberDetailInfo
	for rows.Next() {
		var m MemberDetailInfo
		if err := rows.Scan(&m.Fingerprint, &m.Username, &m.X25519PublicKey, &m.Mode); err != nil {
			return nil, err
		}
		members = append(members, m)
	}
	return members, rows.Err()
}
