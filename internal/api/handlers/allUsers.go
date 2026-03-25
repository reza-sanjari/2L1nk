package handlers

import (
	"2L1nk/internal/hub"
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

func (h *Handler) GetAllUsers(c echo.Context) error {
	// Persistent users come from DB; ephemeral users are hub-only.
	dbUsers, err := h.services.Gate.GetAllUsers()
	if err != nil {
		h.logg.Error("get all users: failed to fetch from DB", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	// Build the list from DB, marking each as online/offline via hub.
	seen := make(map[string]struct{}, len(dbUsers))
	users := make([]hub.UserStatus, 0, len(dbUsers))
	for _, u := range dbUsers {
		seen[u.Fingerprint] = struct{}{}
		users = append(users, hub.UserStatus{
			Username:    u.Username,
			Fingerprint: u.Fingerprint,
			Online:      h.hub.IsUserOnline(u.Fingerprint),
		})
	}

	// Append online ephemeral users (not in DB, so not in seen).
	for _, u := range h.hub.GetUsers() {
		if _, ok := seen[u.Fingerprint]; !ok {
			users = append(users, u)
		}
	}

	return c.JSON(http.StatusOK, users)
}
