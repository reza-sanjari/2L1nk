package handlers

import (
	"2L1nk/internal/hub"
	"encoding/json"
	"fmt"
	"log"

	"github.com/labstack/echo/v4"
	"golang.org/x/net/websocket"
)

type AuthPayload struct {
	SessionID string `json:"Chat-Session-ID"`
	Timestamp int64  `json:"Chat-Timestamp"`
	Signature string `json:"Chat-Signature"`
}

func (h *Handler) Ws(c echo.Context) error {
	websocket.Handler(func(ws *websocket.Conn) {
		defer func() {
			if err := ws.Close(); err != nil {
				log.Printf("error closing websocket: %v", err)
			}
		}()

		// 1. read first message
		var raw []byte
		if err := websocket.Message.Receive(ws, &raw); err != nil {
			log.Println("failed to receive first message:", err)
			return
		}

		// 2. decode envelope
		var msg hub.WSMessageEnvelope
		if err := json.Unmarshal(raw, &msg); err != nil {
			log.Println("invalid message:", err)
			return
		}

		// 3. require auth as first message
		if msg.Type != "auth" {
			log.Println("first message must be auth")
			return
		}

		// 4. decode auth payload
		var auth AuthPayload
		if err := json.Unmarshal(msg.Payload, &auth); err != nil {
			log.Println("invalid auth payload:", err)
			return
		}

		// 5. validate auth
		activeUser, ok := h.Session.Get(auth.SessionID)
		if !ok {
			log.Println("user not active")
			return
		}

		fmt.Printf("user %v is active", activeUser.Username)
		// TODO: validate timestamp + signature here
	}).ServeHTTP(c.Response(), c.Request())

	return nil
}
