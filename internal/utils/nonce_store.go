package utils

import (
	"sync"
	"time"
)

type nonceEntry struct {
	expiresAt time.Time
}

// NonceStore is a thread-safe store that prevents signature replay attacks.
// Each nonce (signature string) may only be accepted once within the TTL window.
type NonceStore struct {
	mu      sync.Mutex
	entries map[string]nonceEntry
	ttl     time.Duration
}

// NewNonceStore creates a NonceStore with the given TTL and starts a background cleanup goroutine.
func NewNonceStore(ttl time.Duration) *NonceStore {
	ns := &NonceStore{
		entries: make(map[string]nonceEntry),
		ttl:     ttl,
	}
	go ns.cleanup()
	return ns
}

// Add records a nonce and returns true if it was not previously seen.
// Returns false if the nonce was already used (replay detected).
func (ns *NonceStore) Add(nonce string) bool {
	ns.mu.Lock()
	defer ns.mu.Unlock()
	if _, exists := ns.entries[nonce]; exists {
		return false
	}
	ns.entries[nonce] = nonceEntry{expiresAt: time.Now().Add(ns.ttl)}
	return true
}

func (ns *NonceStore) cleanup() {
	ticker := time.NewTicker(ns.ttl)
	defer ticker.Stop()
	for range ticker.C {
		ns.mu.Lock()
		now := time.Now()
		for k, v := range ns.entries {
			if now.After(v.expiresAt) {
				delete(ns.entries, k)
			}
		}
		ns.mu.Unlock()
	}
}
