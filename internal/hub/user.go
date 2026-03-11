package hub

import (
	"2L1nk/internal/models"
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

func (U *User) WritePump() error {
	return nil
}

func (U *User) ReadPump() error {
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
