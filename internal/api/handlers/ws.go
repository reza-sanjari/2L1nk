package handlers

import (
	"2L1nk/internal/hub"
	"encoding/json"
	"log"
	"net/http"

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
		h.logg.Debug("websocket closed, user is not authenticated", zap.String("username", activeUser.Username), zap.String("sessionId", activeUser.SessionID))
		return nil
	}

	h.logg.Debug("websocket authenticated", zap.String("username", activeUser.Username), zap.String("fingerprint", activeUser.PublicKeyFingerprint))

	// TODO: validate timestamp + signature here

	newUser := hub.NewUser(activeUser.PublicKeyFingerprint, activeUser.Username, ws, activeUser.Mode, h.logg)

	h.hub.RegisterUser <- newUser

	// start writer
	go func() {
		if err := newUser.WritePump(); err != nil {
			h.logg.Debug("write pump closed", zap.Error(err))
		}
	}()

	// reader blocks until disconnect
	if err := newUser.ReadPump(h.hub.InboundMessages); err != nil {
		h.logg.Debug("read pump closed", zap.Error(err))
	}

	// cleanup
	h.hub.UnregisterUser <- newUser

	return nil
}
