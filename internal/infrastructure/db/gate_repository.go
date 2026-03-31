package db

import (
	"database/sql"
	"fmt"
	"time"

	"2L1nk/internal/gate"
)

type GateRepository struct {
	db *sql.DB
}

func NewGateRepository(db *sql.DB) *GateRepository {
	return &GateRepository{db: db}
}

// InsertToken deactivates all existing active tokens and inserts a new one.
func (r *GateRepository) InsertToken(token string, maxUses int) (*gate.GateTokenRecord, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("insert token: begin tx: %w", err)
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`UPDATE gate_tokens SET is_active = 0 WHERE is_active = 1`); err != nil {
		return nil, fmt.Errorf("insert token: deactivate old: %w", err)
	}

	now := time.Now().Unix()
	res, err := tx.Exec(
		`INSERT INTO gate_tokens (token, max_uses, use_count, is_active, created_at) VALUES (?, ?, 0, 1, ?)`,
		token, maxUses, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert token: insert: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("insert token: last insert id: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("insert token: commit: %w", err)
	}

	return &gate.GateTokenRecord{
		ID:        id,
		Token:     token,
		MaxUses:   maxUses,
		UseCount:  0,
		IsActive:  true,
		CreatedAt: now,
	}, nil
}

// GetActiveToken returns the single active token, or nil if none exists.
func (r *GateRepository) GetActiveToken() (*gate.GateTokenRecord, error) {
	rec := &gate.GateTokenRecord{}
	var isActiveInt int
	err := r.db.QueryRow(
		`SELECT id, token, max_uses, use_count, is_active, created_at FROM gate_tokens WHERE is_active = 1 LIMIT 1`,
	).Scan(&rec.ID, &rec.Token, &rec.MaxUses, &rec.UseCount, &isActiveInt, &rec.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get active token: %w", err)
	}
	rec.IsActive = isActiveInt == 1
	return rec, nil
}

// IncrementUseCount atomically increments use_count for the given ID and returns the new count.
func (r *GateRepository) IncrementUseCount(id int64) (int, error) {
	if _, err := r.db.Exec(
		`UPDATE gate_tokens SET use_count = use_count + 1 WHERE id = ?`, id,
	); err != nil {
		return 0, fmt.Errorf("increment use count: %w", err)
	}
	var newCount int
	if err := r.db.QueryRow(
		`SELECT use_count FROM gate_tokens WHERE id = ?`, id,
	).Scan(&newCount); err != nil {
		return 0, fmt.Errorf("increment use count: read back: %w", err)
	}
	return newCount, nil
}

// UpdateMaxUses sets max_uses for the given token ID.
func (r *GateRepository) UpdateMaxUses(id int64, maxUses int) error {
	if _, err := r.db.Exec(
		`UPDATE gate_tokens SET max_uses = ? WHERE id = ?`, maxUses, id,
	); err != nil {
		return fmt.Errorf("update max uses: %w", err)
	}
	return nil
}

// Close releases the underlying database connection.
func (r *GateRepository) Close() error {
	return r.db.Close()
}

// GetAllTokens returns all tokens ordered by created_at DESC.
func (r *GateRepository) GetAllTokens() ([]gate.GateTokenRecord, error) {
	rows, err := r.db.Query(
		`SELECT id, token, max_uses, use_count, is_active, created_at FROM gate_tokens ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("get all tokens: %w", err)
	}
	defer rows.Close()

	var records []gate.GateTokenRecord
	for rows.Next() {
		var rec gate.GateTokenRecord
		var isActiveInt int
		if err := rows.Scan(&rec.ID, &rec.Token, &rec.MaxUses, &rec.UseCount, &isActiveInt, &rec.CreatedAt); err != nil {
			return nil, fmt.Errorf("get all tokens: scan: %w", err)
		}
		rec.IsActive = isActiveInt == 1
		records = append(records, rec)
	}
	return records, rows.Err()
}
