package db

import (
	"2L1nk/internal/logger"
	"database/sql"

	"go.uber.org/zap"
)

// Setup opens the database, runs migrations, and verifies all expected tables.
// It logs individual table checks at debug level and a single info log on success.
func Setup(path string, logg *logger.Logger) (*sql.DB, error) {
	logg.Info("initializing database", zap.String("path", path))

	database, err := Open(path)
	if err != nil {
		return nil, err
	}

	tables, err := VerifyTables(database)
	if err != nil {
		database.Close()
		return nil, err
	}

	for _, name := range ExpectedTables() {
		if tables[name] {
			logg.Debug("db table ok", zap.String("table", name))
		} else {
			database.Close()
			logg.Fatal("db table missing after migration", zap.String("table", name))
		}
	}

	logg.Info("database ready", zap.String("path", path))
	return database, nil
}
