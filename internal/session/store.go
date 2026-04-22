package session

import (
	"2L1nk/internal/models"
	"crypto/ed25519"
	"sync"
	"time"
)

const sessionTTL = 24 * time.Hour

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
	sessions  map[string]*User
	usernames map[string]bool
	createdAt map[string]time.Time
}

func NewStore() *Store {
	s := &Store{
		sessions:  make(map[string]*User),
		usernames: make(map[string]bool),
		createdAt: make(map[string]time.Time),
	}
	go s.cleanup()
	return s
}

func (s *Store) Add(user *User) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sessions[user.SessionID] = user
	s.usernames[user.Username] = true
	s.createdAt[user.SessionID] = time.Now()
}

func (s *Store) Get(sessionID string) (*User, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if created, ok := s.createdAt[sessionID]; ok {
		if time.Since(created) > sessionTTL {
			return nil, false
		}
	}
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
		delete(s.createdAt, sessionID)
	}
}

func (s *Store) cleanup() {
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for range ticker.C {
		s.mu.Lock()
		now := time.Now()
		for id, created := range s.createdAt {
			if now.Sub(created) > sessionTTL {
				if u, ok := s.sessions[id]; ok {
					delete(s.usernames, u.Username)
					delete(s.sessions, id)
				}
				delete(s.createdAt, id)
			}
		}
		s.mu.Unlock()
	}
}
