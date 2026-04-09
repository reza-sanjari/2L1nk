package server

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"

	"github.com/labstack/echo/v4"

	assets "2L1nk"
	"2L1nk/internal/api"
	"2L1nk/internal/api/handlers"
	"2L1nk/internal/config"
	"2L1nk/internal/session"
	"2L1nk/internal/utils"
)

type Server struct {
	echo *echo.Echo
	cfg  *config.Config
}

// serveFileFS serves a single file from the given fs.FS.
func serveFileFS(c echo.Context, file string, filesystem fs.FS) error {
	f, err := filesystem.Open(file)
	if err != nil {
		return echo.ErrNotFound
	}
	defer f.Close()

	fi, err := f.Stat()
	if err != nil || fi.IsDir() {
		return echo.ErrNotFound
	}

	rs, ok := f.(io.ReadSeeker)
	if !ok {
		return echo.ErrNotFound
	}
	http.ServeContent(c.Response(), c.Request(), fi.Name(), fi.ModTime(), rs)
	return nil
}

func New(cfg *config.Config, h *handlers.Handler, s *session.Store, ns *utils.NonceStore) *Server {
	e := echo.New()

	// Register API routes first (highest priority)
	api.RegisterRoutes(e, h, s, ns)

	// Build sub-filesystems from embedded assets
	webFS, _ := fs.Sub(assets.WebFS, "web")
	pagesFS, _ := fs.Sub(webFS, "pages")

	// Serve static assets (CSS, JS)
	e.StaticFS("/css", echo.MustSubFS(webFS, "css"))
	e.StaticFS("/js", echo.MustSubFS(webFS, "js"))

	// Root page
	e.GET("/", func(c echo.Context) error {
		return serveFileFS(c, "Login.html", pagesFS)
	})

	// Catch-all: map clean URLs to HTML files under web/pages/
	e.GET("/*", func(c echo.Context) error {
		urlPath := c.Param("*")

		// Prevent path traversal
		urlPath = path.Clean(urlPath)
		if strings.Contains(urlPath, "..") {
			return echo.NewHTTPError(http.StatusBadRequest)
		}

		// Try exact HTML file
		if _, err := fs.Stat(pagesFS, urlPath+".html"); err == nil {
			return serveFileFS(c, urlPath+".html", pagesFS)
		}

		// Try directory index
		if _, err := fs.Stat(pagesFS, urlPath+"/index.html"); err == nil {
			return serveFileFS(c, urlPath+"/index.html", pagesFS)
		}

		return echo.NewHTTPError(http.StatusNotFound)
	})

	// Custom 404 handler
	e.HTTPErrorHandler = func(err error, c echo.Context) {
		if c.Response().Committed {
			return
		}

		if he, ok := err.(*echo.HTTPError); ok && he.Code == http.StatusNotFound {
			if serveErr := serveFileFS(c, "404.html", pagesFS); serveErr == nil {
				return
			}
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

func (s *Server) Stop(ctx context.Context) error {
	return s.echo.Shutdown(ctx)
}
