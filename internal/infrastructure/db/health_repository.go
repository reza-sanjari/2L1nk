package db

type HealthRepository struct{}

func NewHealthRepository() *HealthRepository {
	return &HealthRepository{}
}

func (r *HealthRepository) Ping() error {
	// No DB: just report healthy.
	return nil
}
