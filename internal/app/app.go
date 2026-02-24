package app

import (
	"2L1nk/internal/api/handlers"
	"2L1nk/internal/config"
	"2L1nk/internal/hub"
	"2L1nk/internal/infrastructure/db"
	"2L1nk/internal/server"
	"2L1nk/internal/service"
	"2L1nk/internal/session"
)

type App struct {
	server *server.Server
}

func New(cfg *config.Config) *App {
	// 1. Session Store
	sessionStore := session.NewStore()

	// 2. Infrastructure
	healthRepo := db.NewHealthRepository()

	// 3. Services
	healthSvc := service.NewHealthService(healthRepo)

	// 4. Service Container
	services := service.NewContainer(healthSvc)

	// 5. Hub (receives session store)
	mainHub := hub.New(sessionStore)

	// 6. Handler
	handler := handlers.NewHandler(services, mainHub)

	// 7. Server (receives handler + session store for middleware)
	srv := server.New(cfg, handler, sessionStore)

	return &App{server: srv}
}

func (a *App) Start() error {
	return a.server.Start()
}
