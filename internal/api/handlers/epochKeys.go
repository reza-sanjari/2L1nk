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
// Validates against DB state (caller must be key_creator_fp, epoch must match current_epoch,
// and no slots may already exist for this epoch).
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

	h.logg.Debug("submit epoch keys request", zap.String("roomID", roomID), zap.String("callerFP", caller.PublicKeyFingerprint), zap.Int64("epoch", req.Epoch), zap.Int("keyCount", len(req.Keys)))

	// Validate against DB state. If the room is not in DB it may be a live ephemeral
	// room — fall back to hub state in that case.
	room, err := h.services.Room.GetRoomByID(roomID)
	if err != nil {
		h.logg.Error("submit epoch keys: failed to fetch room", zap.String("roomID", roomID), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}

	if room == nil {
		// Ephemeral room — validate against live hub state and skip DB persistence.
		liveRoom := h.hub.GetRoom(roomID)
		if liveRoom == nil {
			h.logg.Debug("submit epoch keys: room not found", zap.String("roomID", roomID))
			return c.JSON(http.StatusNotFound, map[string]string{"error": "room not found"})
		}
		if liveRoom.KeyCreatorFP != caller.PublicKeyFingerprint {
			h.logg.Warn("submit epoch keys: forbidden, caller is not the key creator", zap.String("roomID", roomID), zap.String("callerFP", caller.PublicKeyFingerprint), zap.String("keyCreatorFP", liveRoom.KeyCreatorFP))
			return c.JSON(http.StatusForbidden, map[string]string{"error": "caller is not the key creator for this epoch"})
		}
		if liveRoom.Epoch != req.Epoch {
			h.logg.Debug("submit epoch keys: epoch mismatch (ephemeral)", zap.String("roomID", roomID), zap.Int64("requestedEpoch", req.Epoch), zap.Int64("currentEpoch", liveRoom.Epoch))
			return c.JSON(http.StatusConflict, map[string]string{"error": "epoch mismatch"})
		}
		hubKeys := make([]hub.KeySlotEntry, 0, len(req.Keys))
		for _, entry := range req.Keys {
			decoded, err := base64.StdEncoding.DecodeString(entry.EncryptedKey)
			if err != nil {
				return c.JSON(http.StatusBadRequest, map[string]string{"error": "invalid base64 in encrypted_key for " + entry.RecipientFP})
			}
			hubKeys = append(hubKeys, hub.KeySlotEntry{RecipientFP: entry.RecipientFP, EncryptedKey: decoded})
		}
		h.hub.EpochKeysSubmitted <- hub.EpochKeysSubmittedRequest{RoomID: roomID, Epoch: req.Epoch, Keys: hubKeys}
		h.logg.Info("epoch keys submitted (ephemeral room)", zap.String("roomID", roomID), zap.Int64("epoch", req.Epoch), zap.String("callerFP", caller.PublicKeyFingerprint), zap.Int("keyCount", len(hubKeys)))
		return c.JSON(http.StatusOK, map[string]any{"epoch": req.Epoch})
	}

	if room.KeyCreatorFP != caller.PublicKeyFingerprint {
		h.logg.Warn("submit epoch keys: forbidden, caller is not the key creator", zap.String("roomID", roomID), zap.String("callerFP", caller.PublicKeyFingerprint), zap.String("keyCreatorFP", room.KeyCreatorFP))
		return c.JSON(http.StatusForbidden, map[string]string{"error": "caller is not the key creator for this epoch"})
	}
	if room.CurrentEpoch != req.Epoch {
		h.logg.Debug("submit epoch keys: epoch mismatch", zap.String("roomID", roomID), zap.Int64("requestedEpoch", req.Epoch), zap.Int64("currentEpoch", room.CurrentEpoch))
		return c.JSON(http.StatusConflict, map[string]string{"error": "epoch mismatch"})
	}
	already, err := h.services.Room.HasKeySlots(roomID, req.Epoch)
	if err != nil {
		h.logg.Error("submit epoch keys: failed to check existing slots", zap.String("roomID", roomID), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	if already {
		h.logg.Debug("submit epoch keys: slots already submitted", zap.String("roomID", roomID), zap.Int64("epoch", req.Epoch))
		return c.JSON(http.StatusConflict, map[string]string{"error": "epoch keys already submitted"})
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
		h.logg.Error("submit epoch keys: failed to store key slots", zap.String("roomID", roomID), zap.Int64("epoch", req.Epoch), zap.Error(err))
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "internal server error"})
	}
	h.logg.Debug("submit epoch keys: slots stored in DB", zap.String("roomID", roomID), zap.Int64("epoch", req.Epoch), zap.Int("count", len(slots)))

	// Signal the hub to deliver keys to online members and clear PendingRotation.
	h.hub.EpochKeysSubmitted <- hub.EpochKeysSubmittedRequest{
		RoomID: roomID,
		Epoch:  req.Epoch,
		Keys:   hubKeys,
	}

	h.logg.Info("epoch keys submitted", zap.String("roomID", roomID), zap.Int64("epoch", req.Epoch), zap.String("callerFP", caller.PublicKeyFingerprint), zap.Int("keyCount", len(slots)))
	return c.JSON(http.StatusOK, map[string]any{"epoch": req.Epoch})
}
