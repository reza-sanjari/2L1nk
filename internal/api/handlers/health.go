package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type HealthService interface {
	GetStatus() (map[string]any, error)
}

type HealthHandler struct {
	service HealthService
}

func NewHealthHandler(svc HealthService) *HealthHandler {
	return &HealthHandler{service: svc}
}

func (h *HealthHandler) Health(c echo.Context) error {
	data, err := h.service.GetStatus()
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]any{
			"status": "error",
			"error":  err.Error(),
		})
	}

	return c.JSON(http.StatusOK, data)
}
