package handlers

import (
	"2L1nk/internal/models"
	"crypto/ed25519"
	"errors"
	"net/http"

	"2L1nk/internal/service"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

type gateAuthorizeRequest struct {
	GateToken string            `json:"gateToken"`
	PublicKey ed25519.PublicKey `json:"publicKey"`
	Username  string            `json:"username"`
	Mode      models.UserMode   `json:"mode"`
}

func (h *Handler) GateAuthorize(c echo.Context) error {
	var req gateAuthorizeRequest
	if err := c.Bind(&req); err != nil {
		h.Logg.Error("failed to process request", zap.Error(err))
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
	}

	h.Logg.Debug("authorization request", zap.Any("req", req))

	if req.GateToken == "" || req.PublicKey == nil || req.Username == "" {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "gateToken, publicKey, and username are required",
		})
	}

	if req.Mode != models.UserModeEphemeral && req.Mode != models.UserModePersistent {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid mode",
		})
	}

	result, err := h.Services.Gate.Authorize(service.GateRequest{
		GateToken: req.GateToken,
		PublicKey: req.PublicKey,
		Username:  req.Username,
		Mode:      req.Mode,
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

	h.Logg.Info(
		"user authorized",
		zap.String("username", req.Username),
		zap.String("sessionId", result.SessionID),
	)
	return c.JSON(http.StatusOK, map[string]string{
		"sessionId": result.SessionID,
	})
}
