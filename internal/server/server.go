package server

import (
	"fmt"

	"github.com/labstack/echo/v4"

	"2L1nk/internal/api"
	"2L1nk/internal/config"
)

type Server struct {
	echo *echo.Echo
	cfg  *config.Config
}

func New(cfg *config.Config, handlers *api.Handler) *Server {
	e := echo.New()

	api.RegisterRoutes(e, handlers)

	return &Server{
		echo: e,
		cfg:  cfg,
	}
}

func (s *Server) Start() error {
	return s.echo.Start(fmt.Sprintf(":%d", s.cfg.Port))
}
