package handlers

import (
	"encoding/base64"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
	"2L1nk/internal/session"
	"go.uber.org/zap"
)

// GetRoomKeySlots handles GET /rooms/:room_id/key-slots.
// Returns stored encrypted key slots for the requesting user for a specific room.
// Query params: limit (default 50, max 100), offset (default 0).
func (h *Handler) GetRoomKeySlots(c echo.Context) error {
	roomID := c.Param("room_id")
	caller := c.Get("user").(*session.User)

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

	h.logg.Debug("get room key slots request", zap.String("roomID", roomID), zap.String("callerFP", caller.PublicKeyFingerprint), zap.Int("limit", limit), zap.Int("offset", offset))

	slots, err := h.services.Room.GetKeySlotsByRoom(roomID, caller.PublicKeyFingerprint, limit, offset)
	if err != nil {
		h.logg.Error("get room key slots: failed to fetch slots", zap.String("roomID", roomID), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	if len(slots) == 0 {
		return c.JSON(http.StatusOK, map[string]any{"key_slots": []any{}})
	}

	result := make([]map[string]any, len(slots))
	for i, s := range slots {
		result[i] = map[string]any{
			"room_id":       s.RoomID,
			"epoch":         s.Epoch,
			"encrypted_key": base64.StdEncoding.EncodeToString(s.EncryptedKey),
		}
	}
	return c.JSON(http.StatusOK, map[string]any{"key_slots": result})
}
