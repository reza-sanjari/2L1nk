package db

import (
	"database/sql"
	"fmt"
)

type MessageRecord struct {
	ID         string `json:"id"`
	RoomID     string `json:"room_id"`
	SenderFP   string `json:"sender_fp"`
	Epoch      int64  `json:"epoch"`
	Type       int    `json:"type"`
	Ciphertext string `json:"ciphertext"`
	CreatedAt  int64  `json:"created_at"`
}

type MessageRepository struct {
	db *sql.DB
}

func NewMessageRepository(db *sql.DB) *MessageRepository {
	return &MessageRepository{db: db}
}

func (r *MessageRepository) Save(msg *MessageRecord) error {
	_, err := r.db.Exec(
		`INSERT INTO messages (id, room_id, sender_fp, epoch, type, ciphertext, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		msg.ID, msg.RoomID, msg.SenderFP, msg.Epoch, msg.Type, msg.Ciphertext, msg.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("save message: %w", err)
	}
	return nil
}

// DeleteByRoom removes all messages for a room.
func (r *MessageRepository) DeleteByRoom(roomID string) error {
	_, err := r.db.Exec(`DELETE FROM messages WHERE room_id = ?`, roomID)
	if err != nil {
		return fmt.Errorf("delete messages by room: %w", err)
	}
	return nil
}

// GetByRoom returns messages for a room ordered by creation time, newest first.
func (r *MessageRepository) GetByRoom(roomID string, limit, offset int) ([]*MessageRecord, error) {
	rows, err := r.db.Query(
		`SELECT id, room_id, sender_fp, epoch, type, ciphertext, created_at
		 FROM messages
		 WHERE room_id = ?
		 ORDER BY created_at DESC
		 LIMIT ? OFFSET ?`,
		roomID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("get messages by room: %w", err)
	}
	defer rows.Close()

	var msgs []*MessageRecord
	for rows.Next() {
		m := &MessageRecord{}
		if err := rows.Scan(&m.ID, &m.RoomID, &m.SenderFP, &m.Epoch, &m.Type, &m.Ciphertext, &m.CreatedAt); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	return msgs, rows.Err()
}
