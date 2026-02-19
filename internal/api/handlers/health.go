package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// Health godoc
// @Summary      Health check
// @Description  Returns application health status
// @Tags         health
// @Accept       json
// @Produce      json
// @Success      200  {object}  map[string]string
// @Router       /api/health [get]
func (h *Handler) Health(c echo.Context) error {
	data, err := h.Services.Health.GetStatus()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"status": "error",
			"error":  err.Error(),
		})
	}

	return c.JSON(http.StatusOK, data)
}
