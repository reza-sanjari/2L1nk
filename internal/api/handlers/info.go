package handlers

import (
	"2L1nk/config"
	"net/http"

	"github.com/labstack/echo/v4"
)

func (h *Handler) Info(c echo.Context) error {
	info, err := config.GetInfo()
	if err != nil {
		h.logg.Error("failed to load app info")
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "failed to load info"})
	}
	return c.JSON(http.StatusOK, info)
}
