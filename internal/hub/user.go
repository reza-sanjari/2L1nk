package hub

import (
	"2L1nk/internal/models"
	"sync"

	"golang.org/x/net/websocket"
)

type User struct {
	ID               string `json:"id"`
	Username         string `json:"username"`
	fingerprint      string
	OutGoingMessages chan []byte
	Websocket        *websocket.Conn
	PeerMux          sync.Mutex
	mode             models.UserMode
}

var userIDLock sync.Mutex
var CurrentUserID = 1

func (U *User) WritePump() error {
	return nil
}

func (U *User) ReadPump() error {
	return nil
}

func NewUser() (*User, error) {
	return nil, nil
}
