package db

import (
	"database/sql"
	"fmt"
)

type UserRecord struct {
	Fingerprint string
	PublicKey   string // base64-encoded
	Username    string
	CreatedAt   int64
}

type UserRepository struct {
	db *sql.DB
}

func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// GetByFingerprint returns the user with the given fingerprint, or nil if not found.
func (r *UserRepository) GetByFingerprint(fingerprint string) (*UserRecord, error) {
	row := r.db.QueryRow(
		`SELECT fingerprint, public_key, username, created_at FROM users WHERE fingerprint = ?`,
		fingerprint,
	)
	u := &UserRecord{}
	err := row.Scan(&u.Fingerprint, &u.PublicKey, &u.Username, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get user by fingerprint: %w", err)
	}
	return u, nil
}

// Create inserts a new user row.
func (r *UserRepository) Create(u *UserRecord) error {
	_, err := r.db.Exec(
		`INSERT INTO users (fingerprint, public_key, username, created_at) VALUES (?, ?, ?, ?)`,
		u.Fingerprint, u.PublicKey, u.Username, u.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

// UpdateUsername overwrites the stored username for the given fingerprint.
func (r *UserRepository) UpdateUsername(fingerprint, username string) error {
	_, err := r.db.Exec(
		`UPDATE users SET username = ? WHERE fingerprint = ?`,
		username, fingerprint,
	)
	if err != nil {
		return fmt.Errorf("update username: %w", err)
	}
	return nil
}
