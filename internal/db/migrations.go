package db

import (
	"database/sql"
	"fmt"
)

var expectedTables = []string{
	"users",
	"rooms",
	"room_members",
	"messages",
	"room_key_slots",
	"voice_sessions",
	"voice_participants",
	"gate_tokens",
}

// RunMigrations creates all tables that do not yet exist, inside a single
// transaction so the schema is always consistent.
func RunMigrations(database *sql.DB) error {
	tx, err := database.Begin()
	if err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	defer tx.Rollback()

	statements := []string{
		`CREATE TABLE IF NOT EXISTS users (
			fingerprint TEXT    PRIMARY KEY,
			public_key  TEXT    NOT NULL,
			username    TEXT,
			mode        INTEGER NOT NULL DEFAULT 0,
			created_at  INTEGER NOT NULL
		)`,

		`CREATE TABLE IF NOT EXISTS rooms (
			id              TEXT    PRIMARY KEY,
			current_epoch   INTEGER NOT NULL DEFAULT 0,
			key_creator_fp  TEXT    REFERENCES users(fingerprint),
			created_at      INTEGER NOT NULL
		)`,

		`CREATE TABLE IF NOT EXISTS room_members (
			room_id   TEXT    NOT NULL REFERENCES rooms(id),
			member_fp TEXT    NOT NULL REFERENCES users(fingerprint),
			joined_at INTEGER NOT NULL,
			PRIMARY KEY (room_id, member_fp)
		)`,

		`CREATE TABLE IF NOT EXISTS messages (
			id         TEXT    PRIMARY KEY,
			room_id    TEXT    NOT NULL REFERENCES rooms(id),
			sender_fp  TEXT    NOT NULL,
			epoch      INTEGER NOT NULL,
			type       INTEGER NOT NULL,
			ciphertext BLOB    NOT NULL,
			created_at INTEGER NOT NULL
		)`,

		`CREATE TABLE IF NOT EXISTS room_key_slots (
			room_id       TEXT    NOT NULL REFERENCES rooms(id),
			epoch         INTEGER NOT NULL,
			recipient_fp  TEXT    NOT NULL REFERENCES users(fingerprint),
			encrypted_key BLOB    NOT NULL,
			created_at    INTEGER NOT NULL,
			PRIMARY KEY (room_id, epoch, recipient_fp)
		)`,

		`CREATE TABLE IF NOT EXISTS voice_sessions (
			id         TEXT    PRIMARY KEY,
			room_id    TEXT    NOT NULL REFERENCES rooms(id),
			started_at INTEGER NOT NULL,
			ended_at   INTEGER
		)`,

		`CREATE TABLE IF NOT EXISTS voice_participants (
			session_id TEXT    NOT NULL REFERENCES voice_sessions(id),
			member_fp  TEXT    NOT NULL,
			joined_at  INTEGER NOT NULL,
			left_at    INTEGER,
			muted      INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (session_id, member_fp)
		)`,

		`CREATE TABLE IF NOT EXISTS gate_tokens (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			token      TEXT    UNIQUE NOT NULL,
			created_at INTEGER NOT NULL
		)`,
	}

	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("execute migration statement: %w", err)
		}
	}

	return tx.Commit()
}

// VerifyTables queries sqlite_master and returns the set of tables that exist.
func VerifyTables(database *sql.DB) (map[string]bool, error) {
	rows, err := database.Query(
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`,
	)
	if err != nil {
		return nil, fmt.Errorf("query sqlite_master: %w", err)
	}
	defer rows.Close()

	found := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		found[name] = true
	}
	return found, rows.Err()
}

// ExpectedTables returns the list of tables the schema requires.
func ExpectedTables() []string {
	return expectedTables
}
