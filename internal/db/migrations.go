package db

import (
	"database/sql"
	"fmt"
	"strings"
)

var expectedTables = []string{
	"users",
	"rooms",
	"room_members",
	"messages",
	"room_key_slots",
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
			fingerprint       TEXT    PRIMARY KEY,
			public_key        TEXT    NOT NULL,
			x25519_public_key TEXT    NOT NULL DEFAULT '',
			username          TEXT,
			mode              INTEGER NOT NULL DEFAULT 1,
			created_at        INTEGER NOT NULL
		)`,

		`CREATE TABLE IF NOT EXISTS rooms (
			id              TEXT    PRIMARY KEY,
			name            TEXT,
			current_epoch   INTEGER NOT NULL DEFAULT 0,
			host_fp         TEXT,
			key_creator_fp  TEXT,
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

`CREATE TABLE IF NOT EXISTS gate_tokens (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			token      TEXT    UNIQUE NOT NULL,
			max_uses   INTEGER NOT NULL DEFAULT 0,
			use_count  INTEGER NOT NULL DEFAULT 0,
			is_active  INTEGER NOT NULL DEFAULT 1,
			created_at INTEGER NOT NULL
		)`,
	}

	for _, stmt := range statements {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("execute migration statement: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Column additions/removals for existing databases.
	// ALTER TABLE cannot run inside the CREATE TABLE transaction.
	if err := addColumnIfNotExists(database, "rooms", "name", "TEXT"); err != nil {
		return err
	}
	if err := addColumnIfNotExists(database, "users", "mode", "INTEGER NOT NULL DEFAULT 1"); err != nil {
		return err
	}
	if err := addColumnIfNotExists(database, "users", "x25519_public_key", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := addColumnIfNotExists(database, "rooms", "host_fp", "TEXT"); err != nil {
		return err
	}
	// Seed host_fp from key_creator_fp for existing rooms that have NULL host_fp.
	// Runs as a no-op after migrateRoomsDropKeyCreatorFK (which already seeds it),
	// but handles the case where the FK was already removed before host_fp was added.
	if _, err := database.Exec(
		`UPDATE rooms SET host_fp = key_creator_fp WHERE host_fp IS NULL AND key_creator_fp IS NOT NULL`,
	); err != nil {
		return fmt.Errorf("seed host_fp from key_creator_fp: %w", err)
	}
	if err := migrateRoomsDropKeyCreatorFK(database); err != nil {
		return err
	}
	if err := addColumnIfNotExists(database, "gate_tokens", "max_uses",
		"INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := addColumnIfNotExists(database, "gate_tokens", "use_count",
		"INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	return addColumnIfNotExists(database, "gate_tokens", "is_active",
		"INTEGER NOT NULL DEFAULT 1")
}

// migrateRoomsDropKeyCreatorFK recreates the rooms table without the FK constraint on
// key_creator_fp so that ephemeral users can hold the key creator role without violating
// referential integrity. Safe to run multiple times (no-op if already migrated).
func migrateRoomsDropKeyCreatorFK(database *sql.DB) error {
	var createSQL string
	err := database.QueryRow(
		`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'rooms'`,
	).Scan(&createSQL)
	if err == sql.ErrNoRows {
		return nil // table doesn't exist yet, handled by CREATE TABLE IF NOT EXISTS
	}
	if err != nil {
		return fmt.Errorf("check rooms schema: %w", err)
	}
	if !strings.Contains(strings.ToUpper(createSQL), "REFERENCES") {
		return nil // FK already removed, nothing to do
	}

	// PRAGMA foreign_keys must be toggled outside a transaction.
	if _, err := database.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		return fmt.Errorf("disable foreign keys: %w", err)
	}

	tx, err := database.Begin()
	if err != nil {
		database.Exec(`PRAGMA foreign_keys = ON`)
		return fmt.Errorf("begin rooms migration: %w", err)
	}

	steps := []string{
		`CREATE TABLE rooms_new (
			id              TEXT    PRIMARY KEY,
			name            TEXT,
			current_epoch   INTEGER NOT NULL DEFAULT 0,
			host_fp         TEXT,
			key_creator_fp  TEXT,
			created_at      INTEGER NOT NULL
		)`,
		`INSERT INTO rooms_new SELECT id, name, current_epoch, key_creator_fp, key_creator_fp, created_at FROM rooms`,
		`DROP TABLE rooms`,
		`ALTER TABLE rooms_new RENAME TO rooms`,
	}
	for _, stmt := range steps {
		if _, err := tx.Exec(stmt); err != nil {
			tx.Rollback()
			database.Exec(`PRAGMA foreign_keys = ON`)
			return fmt.Errorf("migrate rooms drop FK: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		database.Exec(`PRAGMA foreign_keys = ON`)
		return fmt.Errorf("commit rooms migration: %w", err)
	}

	if _, err := database.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		return fmt.Errorf("re-enable foreign keys: %w", err)
	}
	return nil
}

// validMigrationTargets is an allowlist of table/column pairs permitted in ALTER TABLE migrations.
// This prevents accidental SQL injection if these helpers are ever called with non-literal arguments.
var validMigrationTargets = map[string]map[string]bool{
	"rooms":       {"name": true, "host_fp": true, "key_creator_fp": true},
	"users":       {"x25519_public_key": true, "mode": true},
	"gate_tokens": {"max_uses": true, "use_count": true, "is_active": true},
}

// addColumnIfNotExists adds a column to a table only if it doesn't already exist.
func addColumnIfNotExists(database *sql.DB, table, column, definition string) error {
	if cols, ok := validMigrationTargets[table]; !ok || !cols[column] {
		return fmt.Errorf("add column: disallowed target %s.%s", table, column)
	}
	var count int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`,
		table, column,
	).Scan(&count); err != nil {
		return fmt.Errorf("check column %s.%s: %w", table, column, err)
	}
	if count > 0 {
		return nil // column already exists
	}
	if _, err := database.Exec(
		fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, table, column, definition),
	); err != nil {
		return fmt.Errorf("add column %s.%s: %w", table, column, err)
	}
	return nil
}

// dropColumnIfExists removes a column from a table only if it exists.
func dropColumnIfExists(database *sql.DB, table, column string) error {
	if cols, ok := validMigrationTargets[table]; !ok || !cols[column] {
		return fmt.Errorf("drop column: disallowed target %s.%s", table, column)
	}
	var count int
	if err := database.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`,
		table, column,
	).Scan(&count); err != nil {
		return fmt.Errorf("check column %s.%s: %w", table, column, err)
	}
	if count == 0 {
		return nil // column doesn't exist, nothing to do
	}
	if _, err := database.Exec(
		fmt.Sprintf(`ALTER TABLE %s DROP COLUMN %s`, table, column),
	); err != nil {
		return fmt.Errorf("drop column %s.%s: %w", table, column, err)
	}
	return nil
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
