package handlers

import (
	"2L1nk/internal/models"
	"2L1nk/internal/utils"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

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
	// Read the raw body so we can both JSON-decode it and hash it for the
	// proof-of-possession canonical.
	var bodyBytes []byte
	if c.Request().Body != nil {
		var err error
		bodyBytes, err = io.ReadAll(c.Request().Body)
		if err != nil {
			h.logg.Error("failed to read gate body", zap.Error(err))
			return c.JSON(http.StatusBadRequest, map[string]string{
				"error": "invalid request body",
			})
		}
	}

	var req gateAuthorizeRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
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
	if len(req.Username) > 50 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "username must be 50 characters or fewer",
		})
	}
	if len(req.X25519PublicKey) != 32 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "x25519PublicKey must be 32 bytes",
		})
	}
	if len(req.PublicKey) != ed25519.PublicKeySize {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "publicKey must be 32 bytes",
		})
	}

	if req.Mode != models.UserModeEphemeral && req.Mode != models.UserModePersistent {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "invalid mode",
		})
	}

	// Proof-of-possession: the caller must prove they hold the Ed25519
	// private key matching req.PublicKey by signing the GATE canonical.
	// This is what closes audit V-01.
	timestampRaw := c.Request().Header.Get("Chat-Timestamp")
	signature := c.Request().Header.Get("Chat-Signature")
	nonce := c.Request().Header.Get("Chat-Nonce")
	if timestampRaw == "" || signature == "" || nonce == "" {
		h.logg.Debug("gate PoP missing headers", zap.String("username", req.Username))
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "missing auth headers",
		})
	}

	bodyHash := utils.HashBodyHex(bodyBytes)
	canonical := utils.GateCanonical(
		timestampRaw,
		nonce,
		strconv.Itoa(int(req.Mode)),
		req.Username,
		base64.StdEncoding.EncodeToString(req.PublicKey),
		base64.StdEncoding.EncodeToString(req.X25519PublicKey),
		bodyHash,
	)

	if err := utils.VerifySignedRequest(
		req.PublicKey,
		canonical,
		timestampRaw,
		signature,
		signature,
		30*time.Second,
		h.nonceStore,
	); err != nil {
		h.logg.Warn("gate proof-of-possession failed", zap.String("username", req.Username), zap.Error(err))
		return c.JSON(utils.AuthErrorStatus(err), map[string]string{
			"error": "proof-of-possession failed",
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

	h.logg.Info("user authorized", zap.String("username", req.Username), zap.String("sessionId", result.SessionID), zap.String("mode", req.Mode.String()))
	return c.JSON(http.StatusOK, map[string]string{
		"sessionId": result.SessionID,
	})
}
