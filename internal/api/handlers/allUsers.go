package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

func (h *Handler) GetAllUsers(c echo.Context) error {
	allUsers := h.hub.GetUsers()
	return c.JSON(http.StatusOK, allUsers)
}
