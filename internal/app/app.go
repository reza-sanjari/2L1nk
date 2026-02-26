package app

import (
	"2L1nk/internal/api/handlers"
	"2L1nk/internal/config"
	"2L1nk/internal/gate"
	"2L1nk/internal/hub"
	"2L1nk/internal/infrastructure/db"
	"2L1nk/internal/server"
	"2L1nk/internal/service"
	"2L1nk/internal/session"
	"fmt"
	"log"
)

type App struct {
	server *server.Server
}

func New(cfg *config.Config) *App {
	// Session Store
	sessionStore := session.NewStore()

	// Infrastructure
	healthRepo := db.NewHealthRepository()

	// Gate
	g, err := gate.New(0)
	if err != nil {
		log.Fatalf("failed to initialize gate: %v", err)
	}
	
	fmt.Printf("Gate key: %s (unlimited uses)\n", g.Key())

	// Services
	healthSvc := service.NewHealthService(healthRepo)
	gateSvc := service.NewGateService(g)

	// Service Container
	services := service.NewContainer(healthSvc, gateSvc)

	// Hub (receives session store)
	mainHub := hub.New(sessionStore)

	// Handler
	handler := handlers.NewHandler(services, mainHub)

	// Server (receives handler + session store for middleware)
	srv := server.New(cfg, handler, sessionStore)

	return &App{server: srv}
}

func (a *App) Start() error {
	return a.server.Start()
}
