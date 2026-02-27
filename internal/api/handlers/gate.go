package handlers

import (
	"crypto/ed25519"
	"errors"
	"net/http"

	"2L1nk/internal/service"
	"2L1nk/internal/session"

	"github.com/labstack/echo/v4"
)

type gateAuthorizeRequest struct {
	GateToken string            `json:"gateToken"`
	PublicKey ed25519.PublicKey `json:"publicKey"`
	Username  string            `json:"username"`
	Mode      int               `json:"mode"`
	Timestamp int               `json:"timestamp"`
	Signature string            `json:"signature"`
}

func (h *Handler) GateAuthorize(c echo.Context) error {
	var req gateAuthorizeRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
	}

	if req.GateToken == "" || req.PublicKey == nil || req.Username == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "gateToken, publicKey, and username are required",
		})
	}

	mode := session.UserModeEphemeral
	if req.Mode == 1 {
		mode = session.UserModePersistent
	}

	result, err := h.Services.Gate.Authorize(service.GateRequest{
		GateToken: req.GateToken,
		PublicKey: req.PublicKey,
		Username:  req.Username,
		Mode:      mode,
	})

	if err != nil {
		if errors.Is(err, service.ErrInvalidGateKey) {
			return c.JSON(http.StatusUnauthorized, map[string]string{
				"error": "invalid gate key",
			})
		}
		if errors.Is(err, service.ErrUsernameTaken) {
			return c.JSON(http.StatusConflict, map[string]string{
				"error": "username already in use",
			})
		}
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "internal server error",
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"sessionId": result.SessionID,
	})
}
