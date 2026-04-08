package handlers

import (
	"2L1nk/internal/hub"
	"2L1nk/internal/models"
	"2L1nk/internal/utils"
	"encoding/base64"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/gorilla/websocket"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
)

type AuthPayload struct {
	SessionID string `json:"Chat-Session-ID"`
	Timestamp int64  `json:"Chat-Timestamp"`
	Signature string `json:"Chat-Signature"`
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

func (h *Handler) Ws(c echo.Context) error {
	ws, err := upgrader.Upgrade(c.Response(), c.Request(), nil)
	if err != nil {
		h.logg.Warn("websocket upgrade failed", zap.Error(err))
		return err
	}
	defer func() {
		if err := ws.Close(); err != nil {
			log.Printf("error closing websocket: %v", err)
		}
	}()

	h.logg.Debug("websocket connection opened by", zap.String("remoteAddr", c.RealIP()))
	if err := ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"info","payload":{"message":"initiated"}}`)); err != nil {
		log.Printf("failed to send initiated message: %v", err)
	}

	// 1. read first message
	_, raw, err := ws.ReadMessage()
	if err != nil {
		h.logg.Debug("websocket closed, failed to read first message", zap.Error(err))
		return nil
	}

	// 2. decode envelope
	var msg hub.WSMessageEnvelope
	if err := json.Unmarshal(raw, &msg); err != nil {
		h.logg.Debug("websocket closed, invalid message", zap.Error(err))
		return nil
	}

	// 3. require auth as first message
	if msg.Type != "auth" {
		h.logg.Debug("websocket closed, invalid first message type", zap.Any("msg", msg))
		return nil
	}

	// 4. decode auth payload
	var auth AuthPayload
	if err := json.Unmarshal(msg.Payload, &auth); err != nil {
		h.logg.Debug("websocket closed, invalid auth payload", zap.Any("msg", msg))
		return nil
	}

	// 5. validate auth
	activeUser, ok := h.session.Get(auth.SessionID)
	if !ok {
		h.logg.Warn("websocket closed: session not found", zap.String("sessionId", auth.SessionID))
		return nil
	}

	h.logg.Debug("websocket authenticated", zap.String("username", activeUser.Username), zap.String("fingerprint", activeUser.PublicKeyFingerprint))

	now := time.Now().Unix()
	if auth.Timestamp < now-30 || auth.Timestamp > now+30 {
		h.logg.Warn("websocket closed: timestamp out of window", zap.String("sessionId", auth.SessionID))
		return nil
	}
	// TODO: add nonce store for full replay prevention

	timestampStr := strconv.FormatInt(auth.Timestamp, 10)
	canonical := utils.WSCanonical(auth.SessionID, timestampStr)
	if err := utils.VerifySignature(activeUser.PublicKey, canonical, auth.Signature); err != nil {
		h.logg.Warn("websocket closed: invalid signature", zap.String("sessionId", auth.SessionID), zap.Error(err))
		return nil
	}

	x25519Key := base64.StdEncoding.EncodeToString(activeUser.X25519PublicKey)
	newUser := hub.NewUser(activeUser.PublicKeyFingerprint, activeUser.Username, x25519Key, ws, activeUser.Mode, h.logg)

	h.hub.RegisterUser <- newUser

	// Case 1: slot this user into any hub rooms they're already a DB member of.
	// Also restore offline rooms where this user is the pending key creator.
	if activeUser.Mode == models.UserModePersistent {
		if dbRooms, err := h.services.Room.GetUserRooms(activeUser.PublicKeyFingerprint); err == nil {
			for _, dbRoom := range dbRooms {
				if h.hub.GetRoom(dbRoom.ID) != nil {
					h.hub.AddToRoom <- hub.AddToRoomRequest{RoomID: dbRoom.ID, User: newUser}
				} else if dbRoom.KeyCreatorFP == activeUser.PublicKeyFingerprint {
					// Room is offline but this user is the key creator — restore it so they
					// can receive the rotation WS if there's a pending rotation.
					hasPending, _ := h.services.Room.HasKeySlots(dbRoom.ID, dbRoom.CurrentEpoch)
					memberKeys, err := h.services.Room.GetMembersWithPublicKeys(dbRoom.ID)
					if err != nil {
						continue
					}
					hubMembers := make([]hub.MemberKeyInfo, len(memberKeys))
					for i, m := range memberKeys {
						hubMembers[i] = hub.MemberKeyInfo{FP: m.Fingerprint, X25519PublicKey: m.X25519PublicKey}
					}
					h.hub.RestoreRoom <- hub.RestoreRoomRequest{
						RoomID:             dbRoom.ID,
						RoomName:           dbRoom.Name,
						HostFP:             dbRoom.HostFP,
						KeyCreatorFP:       dbRoom.KeyCreatorFP,
						Epoch:              dbRoom.CurrentEpoch,
						Members:            hubMembers,
						HasPendingRotation: !hasPending,
					}
				}
			}
		}
	}

	// start writer
	go func() {
		if err := newUser.WritePump(); err != nil {
			h.logg.Debug("write pump closed", zap.Error(err))
		}
	}()

	// reader blocks until disconnect
	if err := newUser.ReadPump(h.hub.InboundMessages); err != nil {
		h.logg.Debug("read pump closed unexpectedly", zap.Error(err))
	}

	// cleanup
	h.hub.UnregisterUser <- newUser

	return nil
}
