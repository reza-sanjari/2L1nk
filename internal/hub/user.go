package hub

import (
	"2L1nk/internal/models"
	"encoding/json"
	"sync"

	"github.com/gorilla/websocket"
)

type User struct {
	Fingerprint      string `json:"fingerprint"`
	Username         string `json:"username"`
	OutGoingMessages chan []byte
	Websocket        *websocket.Conn
	PeerMux          sync.Mutex
	Mode             models.UserMode
}

func (u *User) ReadPump(inbound chan<- WSMessageEnvelope) error {
	for {
		_, message, err := u.Websocket.ReadMessage()
		if err != nil {
			return err
		}

		var envelope WSMessageEnvelope
		if err := json.Unmarshal(message, &envelope); err != nil {
			continue
		}

		envelope.Sender = u

		inbound <- envelope
	}
}

func (u *User) WritePump() error {
	for msg := range u.OutGoingMessages {
		u.PeerMux.Lock()

		err := u.Websocket.WriteMessage(websocket.TextMessage, msg)

		u.PeerMux.Unlock()

		if err != nil {
			return err
		}
	}
	return nil
}

func NewUser(fingerprint string, username string, websocket *websocket.Conn, mode models.UserMode) *User {
	return &User{
		Fingerprint:      fingerprint,
		Username:         username,
		OutGoingMessages: make(chan []byte),
		Websocket:        websocket,
		PeerMux:          sync.Mutex{},
		Mode:             mode,
	}
}
