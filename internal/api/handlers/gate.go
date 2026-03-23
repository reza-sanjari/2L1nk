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
	GateToken       string            `json:"gateToken"`
	PublicKey       ed25519.PublicKey `json:"publicKey"`
	X25519PublicKey []byte            `json:"x25519PublicKey"`
	Username        string            `json:"username"`
	Mode            models.UserMode   `json:"mode"`
}

func (h *Handler) GateAuthorize(c echo.Context) error {
	var req gateAuthorizeRequest
	if err := c.Bind(&req); err != nil {
		h.logg.Error("failed to process request", zap.Error(err))
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid request body",
		})
	}

	h.logg.Debug("session issue request", zap.String("Username", req.Username))

	if req.GateToken == "" || req.PublicKey == nil || req.Username == "" || len(req.X25519PublicKey) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "gateToken, publicKey, x25519PublicKey, and username are required",
		})
	}
	if len(req.X25519PublicKey) != 32 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "x25519PublicKey must be 32 bytes",
		})
	}

	if req.Mode != models.UserModeEphemeral && req.Mode != models.UserModePersistent {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid mode",
		})
	}

	result, err := h.services.Gate.Authorize(service.GateRequest{
		GateToken:       req.GateToken,
		PublicKey:       req.PublicKey,
		X25519PublicKey: req.X25519PublicKey,
		Username:        req.Username,
		Mode:            req.Mode,
	})

	if err != nil {
		if errors.Is(err, service.ErrInvalidGateKey) {
			h.logg.Warn("gate authorization failed: invalid gate token", zap.String("username", req.Username))
			return c.JSON(http.StatusUnauthorized, map[string]string{
				"error": "invalid gate key",
			})
		}
		if errors.Is(err, service.ErrUsernameTaken) {
			h.logg.Debug("gate authorization failed: username taken", zap.String("username", req.Username))
			return c.JSON(http.StatusConflict, map[string]string{
				"error": "username already in use",
			})
		}
		h.logg.Error("gate authorization failed: internal error", zap.String("username", req.Username), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{
			"error": "internal server error",
		})
	}

	h.logg.Info("user authorized", zap.String("username", req.Username), zap.String("sessionId", result.SessionID), zap.Int("mode", int(req.Mode)))
	return c.JSON(http.StatusOK, map[string]string{
		"sessionId": result.SessionID,
	})
}
