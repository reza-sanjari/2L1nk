package app

import (
	"2L1nk/internal/api"
	"2L1nk/internal/config"
	"2L1nk/internal/infrastructure/db"
	"2L1nk/internal/server"
	"2L1nk/internal/service"
)

type App struct {
	server *server.Server
}

func New(cfg *config.Config) *App {
	// Infrastructure
	healthRepo := db.NewHealthRepository()

	// Services
	healthSvc := service.NewHealthService(healthRepo)

	// Handlers
	handlers := api.NewHandler(healthSvc)

	// Server
	srv := server.New(cfg, handlers)

	return &App{
		server: srv,
	}
}

func (a *App) Start() error {
	return a.server.Start()
}
