package handlers

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

func (h *Handler) GetRoomMessages(c echo.Context) error {
	roomID := c.Param("room_id")

	limit := 50
	if l := c.QueryParam("limit"); l != "" {
		if v, err := strconv.Atoi(l); err == nil && v > 0 && v <= 100 {
			limit = v
		}
	}

	offset := 0
	if o := c.QueryParam("offset"); o != "" {
		if v, err := strconv.Atoi(o); err == nil && v >= 0 {
			offset = v
		}
	}

	msgs, err := h.services.Message.GetRoomMessages(roomID, limit, offset)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	if msgs == nil {
		return c.JSON(http.StatusOK, map[string]any{"messages": []any{}})
	}

	return c.JSON(http.StatusOK, map[string]any{"messages": msgs})
}
