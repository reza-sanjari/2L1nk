package app

import (
	"2L1nk/internal/api/handlers"
	"2L1nk/internal/config"
	"2L1nk/internal/db"
	"2L1nk/internal/gate"
	"2L1nk/internal/hub"
	infradb "2L1nk/internal/infrastructure/db"
	"2L1nk/internal/logger"
	"2L1nk/internal/server"
	"2L1nk/internal/service"
	"2L1nk/internal/session"
	"fmt"

	"go.uber.org/zap"
)

type App struct {
	server *server.Server
	logger *logger.Logger
}

func New(cfg *config.Config) *App {
	// Logger
	logg, err := logger.New(logger.Config{
		Level:      "debug", // debug | info | warn | error
		JSON:       false,
		OutputFile: "", //  file path or empty for stdout
	})
	if err != nil {
		panic(err)
	}

	// Database
	database, err := db.Setup(cfg.DBPath, logg)
	if err != nil {
		logg.Fatal("failed to initialize database", zap.Error(err))
	}

	// Session Store
	sessionStore := session.NewStore()

	// Infrastructure
	healthRepo := infradb.NewHealthRepository(database)
	roomRepo := infradb.NewRoomRepository(database)
	msgRepo := infradb.NewMessageRepository(database)
	userRepo := infradb.NewUserRepository(database)

	// Gate
	g, err := gate.New(0)
	if err != nil {
		logg.Fatal("failed to initialize gate", zap.Error(err))
	}

	logg.Info(fmt.Sprintf("gate initialized: %s %s", g.Key(), "unlimited"))

	// Services
	healthSvc := service.NewHealthService(healthRepo, logg)
	gateSvc := service.NewGateService(g, sessionStore, userRepo, logg)
	roomSvc := service.NewRoomService(roomRepo, logg)
	msgSvc := service.NewMessageService(msgRepo, roomRepo, logg)

	// Service Container
	services := service.NewContainer(healthSvc, gateSvc, roomSvc, msgSvc)

	// Hub
	mainHub := hub.New(sessionStore, logg)
	go mainHub.Run()

	// Event consumer: wires hub events to services for DB persistence
	startEventConsumer(mainHub.Events, roomSvc, msgSvc, logg)

	// Handler
	handler := handlers.NewHandler(services, mainHub, sessionStore, logg)

	// Server
	srv := server.New(cfg, handler, sessionStore)

	return &App{
		server: srv,
		logger: logg,
	}
}

func (a *App) Start() error {
	defer a.logger.Sync()
	return a.server.Start()
}
