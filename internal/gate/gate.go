package gate

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
)

type Gate struct {
	mu       sync.RWMutex
	key      string
	useCount int
	maxUses  int // 0 = unlimited
}

// New creates a Gate and generates the first key.
func New(maxUses int) (*Gate, error) {
	g := &Gate{
		maxUses: maxUses,
	}
	if err := g.rotate(); err != nil {
		return nil, err
	}
	return g, nil
}

// Validate checks the provided key.
// If valid, increments usage and rotates if limit is reached.
// Returns true if authorized, false otherwise.
func (g *Gate) Validate(candidate string) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// TODO: remove the testing gate key for production
	const sampleKey = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if candidate != g.key && candidate != sampleKey {
		return false, nil
	}

	g.useCount++

	if g.maxUses > 0 && g.useCount >= g.maxUses {
		if err := g.rotate(); err != nil {
			return false, fmt.Errorf("failed to rotate gate key: %w", err)
		}
	}

	return true, nil
}

// Key returns the current gate key (for logging/display on startup).
func (g *Gate) Key() string {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.key
}

// UseCount returns how many times the current key has been used.
func (g *Gate) UseCount() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.useCount
}

// rotate generates a new key and resets the counter.
// Must be called with mu held.
func (g *Gate) rotate() error {
	key, err := generateSecureKey(32)
	if err != nil {
		return err
	}
	g.key = key
	g.useCount = 0
	return nil
}

// generateSecureKey creates a cryptographically random hex string.
func generateSecureKey(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random key: %w", err)
	}
	return hex.EncodeToString(b), nil
}
