package server

import (
	"2L1nk/internal/session"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/labstack/echo/v4"

	"2L1nk/internal/api"
	"2L1nk/internal/api/handlers"
	"2L1nk/internal/config"
)

type Server struct {
	echo *echo.Echo
	cfg  *config.Config
}

func New(cfg *config.Config, h *handlers.Handler, s *session.Store) *Server {
	e := echo.New()

	// Register API routes first (highest priority)
	api.RegisterRoutes(e, h, s)

	// Serve static assets (CSS, JS, images, etc.)
	e.Static("/css", "web/css")
	e.Static("/js", "web/js")

	// Root page
	e.GET("/", func(c echo.Context) error {
		return c.File("web/pages/login.html")
	})

	// Catch-all: map clean URLs to HTML files under web/pages/
	e.GET("/*", func(c echo.Context) error {
		urlPath := c.Param("*") // e.g. "login", "admin/settings"

		// Prevent path traversal
		urlPath = filepath.Clean(urlPath)
		if strings.Contains(urlPath, "..") {
			return echo.NewHTTPError(http.StatusBadRequest)
		}

		// Build the candidate file path
		candidate := filepath.Join("web", "pages", urlPath+".html")

		// Check if the file exists
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return c.File(candidate)
		}

		// Also check if it's a directory with an index.html
		// e.g. /admin → web/pages/admin/index.html
		candidate = filepath.Join("web", "pages", urlPath, "index.html")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return c.File(candidate)
		}

		// Nothing found → 404
		return echo.NewHTTPError(http.StatusNotFound)
	})

	// Custom 404 handler
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}

		if he, ok := err.(*echo.HTTPError); ok && he.Code == http.StatusNotFound {
			_ = c.File("web/pages/404.html")
			return
		}

		e.DefaultHTTPErrorHandler(err, c)
	}

	return &Server{
		echo: e,
		cfg:  cfg,
	}
}

func (s *Server) Start() error {
	return s.echo.Start(fmt.Sprintf(":%d", s.cfg.Port))
}
