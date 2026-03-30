package app

import (
	"context"
	"fmt"

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

	"go.uber.org/zap"
)

type App struct {
	server *server.Server
	logger *logger.Logger
}

// New wires up the application. logFile is an optional path for log output;
// when non-empty, stdout logging is suppressed (server runs as a subprocess).
func New(cfg *config.Config, g *gate.Gate, logFile string) *App {
	logg, err := logger.New(logger.Config{
		Level:          "debug",
		JSON:           false,
		OutputFile:     logFile,
		SuppressStdout: logFile != "",
	})
	if err != nil {
		panic(err)
	}

	database, err := db.Setup(cfg.DBPath, logg)
	if err != nil {
		logg.Fatal("failed to initialize database", zap.Error(err))
	}

	sessionStore := session.NewStore()

	healthRepo := infradb.NewHealthRepository(database)
	roomRepo := infradb.NewRoomRepository(database)
	msgRepo := infradb.NewMessageRepository(database)
	userRepo := infradb.NewUserRepository(database)

	logg.Info(fmt.Sprintf("gate initialized: %s unlimited", g.Key()))

	healthSvc := service.NewHealthService(healthRepo, logg)
	gateSvc := service.NewGateService(g, sessionStore, userRepo, logg)
	roomSvc := service.NewRoomService(roomRepo, logg)
	msgSvc := service.NewMessageService(msgRepo, roomRepo, logg)

	services := service.NewContainer(healthSvc, gateSvc, roomSvc, msgSvc)

	mainHub := hub.New(sessionStore, logg)
	go mainHub.Run()

	startEventConsumer(mainHub, roomSvc, msgSvc, logg)

	handler := handlers.NewHandler(services, mainHub, sessionStore, logg)
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

func (a *App) Stop(ctx context.Context) error {
	defer a.logger.Sync()
	return a.server.Stop(ctx)
}
