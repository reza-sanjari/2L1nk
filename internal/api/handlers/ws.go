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
		log.Printf("websocket upgrade failed: %v", err)
		return err
	}
	defer func() {
		if err := ws.Close(); err != nil {
			log.Printf("error closing websocket: %v", err)
		}
	}()

	h.Logg.Debug("websocket connection opened")
	if err := ws.WriteMessage(websocket.TextMessage, []byte(`{"type":"info","payload":{"message":"initiated"}}`)); err != nil {
		log.Printf("failed to send initiated message: %v", err)
	}

	h.Logg.Debug("waiting for first websocket message")

	// 1. read first message
	_, raw, err := ws.ReadMessage()
	if err != nil {
		log.Println("failed to receive first message:", err)
		return nil
	}

	h.Logg.Debug("raw websocket message received", zap.ByteString("raw", raw))

	// 2. decode envelope
	var msg hub.WSMessageEnvelope
	if err := json.Unmarshal(raw, &msg); err != nil {
		log.Println("invalid message:", err)
		return nil
	}

	h.Logg.Debug("received message", zap.Any("msg", msg))

	// 3. require auth as first message
	if msg.Type != "auth" {
		log.Println("first message must be auth")
		return nil
	}

	// 4. decode auth payload
	var auth AuthPayload
	if err := json.Unmarshal(msg.Payload, &auth); err != nil {
		log.Println("invalid auth payload:", err)
		return nil
	}

	// 5. validate auth
	activeUser, ok := h.Session.Get(auth.SessionID)
	if !ok {
		log.Println("user not active")
		return nil
	}

	h.Logg.Debug("active user authenticated through websocket", zap.String("username", activeUser.Username))
	// TODO: validate timestamp + signature here

	newUser := hub.NewUser(activeUser.PublicKeyFingerprint, activeUser.Username, ws, activeUser.Mode)

	h.Hub.RegisterUser <- newUser
	
	// start writer
	go func() {
		if err := newUser.WritePump(); err != nil {
			h.Logg.Debug("write pump closed", zap.Error(err))
		}
	}()

	// reader blocks until disconnect
	if err := newUser.ReadPump(h.Hub.InboundMessages); err != nil {
		h.Logg.Debug("read pump closed", zap.Error(err))
	}

	// cleanup
	h.Hub.UnregisterUser <- newUser

	return nil
}
