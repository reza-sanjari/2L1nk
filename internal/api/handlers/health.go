package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

func (h *Handler) Health(c echo.Context) error {
	data, err := h.services.Health.GetStatus()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"status": "error",
			"error":  err.Error(),
		})
	}

	return c.JSON(http.StatusOK, data)
}
