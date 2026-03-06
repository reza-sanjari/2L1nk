package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

func (h *Handler) NewRoom(c echo.Context) error {
	return c.String(http.StatusCreated, "New Room handler called")
}
