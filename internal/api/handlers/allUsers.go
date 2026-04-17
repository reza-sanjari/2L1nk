package handlers

import (
	"2L1nk/internal/hub"
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

func (h *Handler) GetAllUsers(c echo.Context) error {
	dbUsers, err := h.services.Gate.GetAllUsers()
	if err != nil {
		h.logg.Error("get all users: failed to fetch from DB", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	users := make([]hub.UserStatus, 0, len(dbUsers))
	for _, u := range dbUsers {
		users = append(users, hub.UserStatus{
			Username:    u.Username,
			Fingerprint: u.Fingerprint,
			Online:      h.hub.IsUserOnline(u.Fingerprint),
		})
	}

	return c.JSON(http.StatusOK, users)
}
