package db

import (
	"database/sql"
	"fmt"
)

type RoomRecord struct {
	ID           string
	Name         string
	CurrentEpoch int64
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
	var keyCreatorFP interface{}
	if room.KeyCreatorFP != "" {
		keyCreatorFP = room.KeyCreatorFP
	}
	_, err := r.db.Exec(
		`INSERT INTO rooms (id, name, current_epoch, key_creator_fp, created_at) VALUES (?, ?, ?, ?, ?)`,
		room.ID, room.Name, room.CurrentEpoch, keyCreatorFP, room.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create room: %w", err)
	}
	return nil
}

// GetByID returns the room with the given ID, or nil if not found.
func (r *RoomRepository) GetByID(roomID string) (*RoomRecord, error) {
	row := r.db.QueryRow(
		`SELECT id, name, current_epoch, key_creator_fp, created_at FROM rooms WHERE id = ?`,
		roomID,
	)
	rec := &RoomRecord{}
	var keyCreatorFP sql.NullString
	err := row.Scan(&rec.ID, &rec.Name, &rec.CurrentEpoch, &keyCreatorFP, &rec.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get room by id: %w", err)
	}
	rec.KeyCreatorFP = keyCreatorFP.String
	return rec, nil
}

// GetRoomsByMember returns all rooms a user is a member of.
func (r *RoomRepository) GetRoomsByMember(fp string) ([]*RoomRecord, error) {
	rows, err := r.db.Query(
		`SELECT r.id, r.name, r.current_epoch, r.key_creator_fp, r.created_at
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
		var keyCreatorFP sql.NullString
		if err := rows.Scan(&rec.ID, &rec.Name, &rec.CurrentEpoch, &keyCreatorFP, &rec.CreatedAt); err != nil {
			return nil, err
		}
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

// UpdateHost sets a new key_creator_fp for the room.
func (r *RoomRepository) UpdateHost(roomID, newHostFP string) error {
	_, err := r.db.Exec(
		`UPDATE rooms SET key_creator_fp = ? WHERE id = ?`,
		newHostFP, roomID,
	)
	if err != nil {
		return fmt.Errorf("update room host: %w", err)
	}
	return nil
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
