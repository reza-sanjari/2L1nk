package handlers

import (
	"2L1nk/internal/hub"
	"2L1nk/internal/session"
	"net/http"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

type CreateRoomRequest struct {
	GroupName string `json:"groupName"`
}

func (h *Handler) NewRoom(c echo.Context) error {
	var req CreateRoomRequest

	if err := c.Bind(&req); err != nil {
		return c.JSON(400, map[string]string{"error": "invalid body"})
	}

	groupName := req.GroupName
	if groupName == "" || len(groupName) > 100 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "room name must be between 1 and 100 characters"})
	}
	caller := c.Get("user").(*session.User)

	h.logg.Debug("create room request", zap.String("groupName", groupName), zap.String("callerFP", caller.PublicKeyFingerprint))

	respChan := make(chan string)

	h.hub.RegisterRoom <- hub.CreateRoomRequest{
		Host:         caller,
		GroupName:    groupName,
		ResponseChan: respChan,
	}

	roomID := <-respChan
	if roomID == "" {
		h.logg.Warn("room creation failed: hub rejected request (caller not connected via WS?)", zap.String("callerFP", caller.PublicKeyFingerprint))
		return c.JSON(http.StatusInternalServerError, "Room creation failed")
	}

	h.logg.Info("room created", zap.String("roomID", roomID), zap.String("callerFP", caller.PublicKeyFingerprint), zap.String("groupName", groupName))
	return c.JSON(http.StatusCreated, map[string]any{
		"room_id": roomID,
	})
}
