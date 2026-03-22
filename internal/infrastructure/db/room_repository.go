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
