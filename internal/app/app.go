package app

import (
	"2L1nk/internal/api/handlers"
	"2L1nk/internal/config"
	"2L1nk/internal/infrastructure/db"
	"2L1nk/internal/server"
	"2L1nk/internal/service"
)

type App struct {
	server *server.Server
}

func New(cfg *config.Config) *App {
	// 1. Infrastructure
	healthRepo := db.NewHealthRepository()

	// 2. Services
	healthSvc := service.NewHealthService(healthRepo)

	// 3. Service Container
	services := service.NewContainer(healthSvc)

	// 4. Hub (when implemented)
	// hub := hub.New()

	// 5. Handler (inject container + hub)
	handler := handlers.NewHandler(services)

	// 6. Server
	srv := server.New(cfg, handler)

	return &App{
		server: srv,
	}
}

func (a *App) Start() error {
	return a.server.Start()
}
