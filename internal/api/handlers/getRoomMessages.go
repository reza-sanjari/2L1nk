package handlers

import (
	"2L1nk/internal/session"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

func (h *Handler) GetRoomMessages(c echo.Context) error {
	user := c.Get("user").(*session.User)
	roomID := c.Param("room_id")

	members, err := h.services.Room.GetRoomMembers(roomID)
	if err != nil {
		h.logg.Error("get room messages: failed to check membership", zap.String("roomID", roomID), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	isMember := false
	for _, fp := range members {
		if fp == user.PublicKeyFingerprint {
			isMember = true
			break
		}
	}
	if !isMember {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "not a member of this room"})
	}

	limit := 50
	if l := c.QueryParam("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}

	offset := 0
	if o := c.QueryParam("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 && v <= 1000000 {
			offset = v
		}
	}

	h.logg.Debug("get room messages request", zap.String("roomID", roomID), zap.Int("limit", limit), zap.Int("offset", offset))

	msgs, err := h.services.Message.GetRoomMessages(roomID, limit, offset)
	if err != nil {
		h.logg.Error("get room messages: failed to fetch messages", zap.String("roomID", roomID), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	if msgs == nil {
		h.logg.Debug("get room messages: no messages found", zap.String("roomID", roomID))
		return c.JSON(http.StatusOK, map[string]any{"messages": []any{}})
	}

	h.logg.Debug("get room messages: returning messages", zap.String("roomID", roomID), zap.Int("count", len(msgs)))
	return c.JSON(http.StatusOK, map[string]any{"messages": msgs})
}
