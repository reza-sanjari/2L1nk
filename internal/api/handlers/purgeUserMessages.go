package handlers

import (
	"net/http"

	"2L1nk/internal/hub"
	"2L1nk/internal/session"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

// PurgeUserMessages deletes all DB messages sent by the authenticated user and
// notifies active rooms to delete them locally. Returns {"deleted": N} where N
// is the number of messages removed; N=0 means the DB was already empty.
func (h *Handler) PurgeUserMessages(c echo.Context) error {
	caller := c.Get("user").(*session.User)
	fp := caller.PublicKeyFingerprint

	count, err := h.services.Message.PurgeUserMessages(fp)
	if err != nil {
		h.logg.Error("purge user messages: db error", zap.String("fp", fp), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	h.logg.Info("purge user messages", zap.String("fp", fp), zap.Int64("deleted", count))

	if count > 0 {
		h.hub.PurgeUserMessages <- hub.PurgeRequest{SenderFP: fp}
	}

	return c.JSON(http.StatusOK, map[string]any{"deleted": count})
}
