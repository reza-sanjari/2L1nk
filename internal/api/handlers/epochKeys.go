package handlers

import (
	"2L1nk/internal/hub"
	infradb "2L1nk/internal/infrastructure/db"
	"2L1nk/internal/session"
	"encoding/base64"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

type epochKeyEntry struct {
	RecipientFP  string `json:"recipient_fp"`
	EncryptedKey string `json:"encrypted_key"` // base64-encoded
}

type submitEpochKeysRequest struct {
	Epoch int64           `json:"epoch"`
	Keys  []epochKeyEntry `json:"keys"`
}

// SubmitEpochKeys handles POST /rooms/:room_id/epoch-keys.
// Called by the key creator after generating and encrypting the new room key for each member.
func (h *Handler) SubmitEpochKeys(c echo.Context) error {
	roomID := c.Param("room_id")
	caller := c.Get("user").(*session.User)

	var req submitEpochKeysRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid request body"})
	}
	if len(req.Keys) == 0 {
		return c.JSON(http.StatusBadRequest, map[string]string{"error": "keys must not be empty"})
	}

	// Validate against hub state (room must exist and caller must be the pending key creator).
	pending := h.hub.GetPendingRotation(roomID)
	if pending == nil {
		return c.JSON(http.StatusConflict, map[string]string{"error": "no pending rotation for this room"})
	}
	if pending.KeyCreatorFP != caller.PublicKeyFingerprint {
		return c.JSON(http.StatusForbidden, map[string]string{"error": "caller is not the key creator for this epoch"})
	}
	if pending.Epoch != req.Epoch {
		return c.JSON(http.StatusConflict, map[string]string{"error": "epoch mismatch"})
	}

	// Decode and store key slots for persistent members.
	now := time.Now().Unix()
	slots := make([]infradb.KeySlotRecord, 0, len(req.Keys))
	hubKeys := make([]hub.KeySlotEntry, 0, len(req.Keys))

	for _, entry := range req.Keys {
		decoded, err := base64.StdEncoding.DecodeString(entry.EncryptedKey)
		if err != nil {
			return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid base64 in encrypted_key for " + entry.RecipientFP})
		}
		slots = append(slots, infradb.KeySlotRecord{
			RoomID:       roomID,
			Epoch:        req.Epoch,
			RecipientFP:  entry.RecipientFP,
			EncryptedKey: decoded,
			CreatedAt:    now,
		})
		hubKeys = append(hubKeys, hub.KeySlotEntry{
			RecipientFP:  entry.RecipientFP,
			EncryptedKey: decoded,
		})
	}

	// StoreKeySlots silently skips ephemeral recipients (not in users table).
	if err := h.services.Room.StoreKeySlots(slots); err != nil {
		h.logg.Error("failed to store epoch key slots", zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	// Signal the hub to deliver keys to online members and clear PendingRotation.
	h.hub.EpochKeysSubmitted <- hub.EpochKeysSubmittedRequest{
		RoomID: roomID,
		Epoch:  req.Epoch,
		Keys:   hubKeys,
	}

	return c.JSON(http.StatusOK, map[string]any{"epoch": req.Epoch})
}
