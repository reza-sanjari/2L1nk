package session

import (
	"2L1nk/internal/models"
	"crypto/ed25519"
	"sync"
)

type User struct {
	SessionID            string
	PublicKey            ed25519.PublicKey
	X25519PublicKey      []byte // raw 32-byte X25519 public key
	PublicKeyFingerprint string
	Username             string
	Mode                 models.UserMode // "ephemeral" or "persistent"
}

type Store struct {
	mu        sync.RWMutex
	sessions  map[string]*User // sessionID → User
	usernames map[string]bool  // username → taken
}

func NewStore() *Store {
	return &Store{
		sessions:  make(map[string]*User),
		usernames: make(map[string]bool),
	}
}

func (s *Store) Add(user *User) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[user.SessionID] = user
	s.usernames[user.Username] = true
}

func (s *Store) Get(sessionID string) (*User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	u, ok := s.sessions[sessionID]
	return u, ok
}

func (s *Store) UsernameExists(username string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.usernames[username]
}

func (s *Store) Remove(sessionID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if u, ok := s.sessions[sessionID]; ok {
		delete(s.usernames, u.Username)
		delete(s.sessions, sessionID)
	}
}
