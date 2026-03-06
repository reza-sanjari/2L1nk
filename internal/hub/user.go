package hub

import (
	"2L1nk/internal/models"
	"sync"

	"golang.org/x/net/websocket"
)

type User struct {
	Fingerprint      string `json:"fingerprint"`
	Username         string `json:"username"`
	OutGoingMessages chan []byte
	Websocket        *websocket.Conn
	PeerMux          sync.Mutex
	mode             models.UserMode
}

func (U *User) WritePump() error {
	return nil
}

func (U *User) ReadPump() error {
	return nil
}

func NewUser() (*User, error) {
	return nil, nil
}
