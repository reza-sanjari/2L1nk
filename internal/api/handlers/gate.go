package handlers

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type gateAuthorizeRequest struct {
	GateToken string `json:"gateToken"`
	PublicKey string `json:"publicKey"`
	Username  string `json:"username"`
	Mode      string `json:"mode"`
	Timestamp int    `json:"timestamp"`
	Signature string `json:"signature"`
}

type gateAuthorizeResponse struct {
	SessionID string `json:"sessionId"`
}

func (h *Handler) GateAuthorize(c echo.Context) error {
	var req gateAuthorizeRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
	}

	if req.GateToken == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "gateToken is required",
		})
	}

	authorized, err := h.Services.Gate.Authorize(req.GateToken)
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "internal server error",
		})
	}

	if !authorized {
		return c.JSON(http.StatusUnauthorized, map[string]string{
			"error": "invalid gate key",
		})
	}

	// TODO: once session store exists, create session here
	// and return the session ID. For now, return authorized.
	return c.JSON(http.StatusOK, map[string]string{
		"authorized": "true",
	})
}
