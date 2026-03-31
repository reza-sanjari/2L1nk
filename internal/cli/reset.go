package cli

import (
	"2L1nk/internal/db"
	"2L1nk/internal/logger"
)

// resetDatabase re-runs migrations on a fresh DB file.
func resetDatabase(dbPath string) error {
	logg, err := logger.New(logger.Config{
		Level:          "error",
		SuppressStdout: true,
	})
	if err != nil {
		return err
	}
	defer logg.Sync()

	database, err := db.Setup(dbPath, logg)
	if err != nil {
		return err
	}
	return database.Close()
}
