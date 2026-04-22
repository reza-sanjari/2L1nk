package db

import "database/sql"

type HealthRepository struct {
	db *sql.DB
}

func NewHealthRepository(db *sql.DB) *HealthRepository {
	return &HealthRepository{db: db}
}

func (r *HealthRepository) Ping() error {
	return r.db.Ping()
}
