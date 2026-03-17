package hub

import (
	"2L1nk/internal/logger"
	"2L1nk/internal/models"
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

type User struct {
	logg             *logger.Logger
	Fingerprint      string `json:"fingerprint"`
	Username         string `json:"username"`
	OutGoingMessages chan []byte
	Websocket        *websocket.Conn
	PeerMux          sync.Mutex
	Mode             models.UserMode
}

func (u *User) ReadPump(inbound chan<- WSMessageEnvelope) error {
	for {
		u.logg.Debug("read pump called for user", zap.String("username", u.Username), zap.String("fingerprint", u.Fingerprint))
		_, message, err := u.Websocket.ReadMessage()
		if err != nil {
			u.logg.Error("websocket closed, failed to read message", zap.Error(err))
			return err
		}

		var envelope WSMessageEnvelope
		if err := json.Unmarshal(message, &envelope); err != nil {
			u.logg.Error(
				"failed to unmarshal websocket message",
				zap.Error(err),
				zap.ByteString("raw_message", message),
			)
			return err
		}

		envelope.Sender = u
		inbound <- envelope
	}
}

func (u *User) WritePump() error {
	for msg := range u.OutGoingMessages {
		u.logg.Debug("write pump called for user", zap.String("username", u.Username), zap.String("fingerprint", u.Fingerprint))
		u.PeerMux.Lock()

		err := u.Websocket.WriteMessage(websocket.TextMessage, msg)

		u.PeerMux.Unlock()

		if err != nil {
			return err
		}
	}
	return nil
}

func NewUser(fingerprint string, username string, websocket *websocket.Conn, mode models.UserMode, logg *logger.Logger) *User {
	return &User{
		logg:             logg,
		Fingerprint:      fingerprint,
		Username:         username,
		OutGoingMessages: make(chan []byte, 32),
		Websocket:        websocket,
		PeerMux:          sync.Mutex{},
		Mode:             mode,
	}
}
