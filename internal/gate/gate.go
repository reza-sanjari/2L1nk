package gate

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"

	"go.uber.org/zap"
)

// GateTokenRecord is a snapshot of one row from gate_tokens.
type GateTokenRecord struct {
	ID        int64
	Token     string
	MaxUses   int
	UseCount  int
	IsActive  bool
	CreatedAt int64
}

// GateRepository is the persistence contract for gate token lifecycle.
// A nil repo means the gate runs in-memory mode (no DB).
type GateRepository interface {
	// InsertToken deactivates all existing tokens and inserts a new active one.
	InsertToken(token string, maxUses int) (*GateTokenRecord, error)
	// GetActiveToken returns the single active token, or nil if none exists.
	GetActiveToken() (*GateTokenRecord, error)
	// IncrementUseCount atomically increments use_count for the given ID and returns the new count.
	IncrementUseCount(id int64) (int, error)
	// UpdateMaxUses sets max_uses for the given token ID.
	UpdateMaxUses(id int64, maxUses int) error
	// GetAllTokens returns all tokens ordered by created_at DESC.
	GetAllTokens() ([]GateTokenRecord, error)
	// Close releases the underlying database connection.
	Close() error
}

type Gate struct {
	mu       sync.RWMutex
	repo     GateRepository // nil = in-memory mode
	key      string
	keyID    int64 // DB row ID of current active token; 0 in in-memory mode
	useCount int
	maxUses  int // 0 = unlimited
	log      *zap.Logger
}

// New creates a Gate and generates the first key (in-memory).
func New(maxUses int) (*Gate, error) {
	g := &Gate{maxUses: maxUses}
	if err := g.rotate(); err != nil {
		return nil, err
	}
	return g, nil
}

// SetRepo attaches a repository and syncs state with the DB.
// If an active token exists in DB it becomes the current key.
// If not, the current in-memory key is seeded into DB.
// After this call all operations are DB-backed.
func (g *Gate) SetRepo(repo GateRepository) error {
	g.mu.Lock()
	defer g.mu.Unlock()

	active, err := repo.GetActiveToken()
	if err != nil {
		return fmt.Errorf("gate SetRepo: get active token: %w", err)
	}

	if active != nil {
		g.key = active.Token
		g.keyID = active.ID
		g.useCount = active.UseCount
		g.maxUses = active.MaxUses
	} else {
		rec, err := repo.InsertToken(g.key, g.maxUses)
		if err != nil {
			return fmt.Errorf("gate SetRepo: seed initial token: %w", err)
		}
		g.keyID = rec.ID
	}

	g.repo = repo
	return nil
}

// Repo returns the attached repository (nil in in-memory mode).
func (g *Gate) Repo() GateRepository {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.repo
}

// refreshActiveLocked syncs the active token from DB into the in-memory cache.
// Must be called with mu held. Errors are silently ignored so callers can
// fall back to the cached key.
func (g *Gate) refreshActiveLocked() {
	if g.repo == nil {
		return
	}
	active, err := g.repo.GetActiveToken()
	if err != nil || active == nil {
		return
	}
	g.key = active.Token
	g.keyID = active.ID
	g.useCount = active.UseCount
	g.maxUses = active.MaxUses
}

// Validate checks the provided key.
// If valid, increments usage and rotates if the limit is reached.
func (g *Gate) Validate(candidate string) (bool, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Sync with DB so key changes made via the CLI take effect immediately
	// without requiring a server restart.
	g.refreshActiveLocked()

	if candidate != g.key {
		return false, nil
	}

	if g.repo != nil {
		newCount, err := g.repo.IncrementUseCount(g.keyID)
		if err != nil {
			return false, fmt.Errorf("gate validate: increment use count: %w", err)
		}
		g.useCount = newCount
	} else {
		g.useCount++
	}

	if g.maxUses > 0 && g.useCount >= g.maxUses {
		if err := g.rotateWithRepo(); err != nil {
			return false, fmt.Errorf("gate validate: auto-rotate: %w", err)
		}
		if g.log != nil {
			g.log.Info("gate key auto-rotated after reaching max uses", zap.String("new_key", g.key))
		}
	}

	return true, nil
}

// Key returns the current gate key.
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

// MaxUses returns the current max-uses limit (0 = unlimited).
func (g *Gate) MaxUses() int {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.maxUses
}

// SetLogger attaches a zap logger to the gate for key lifecycle events.
func (g *Gate) SetLogger(log *zap.Logger) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.log = log
}

// Rotate generates a new key and resets the counter.
func (g *Gate) Rotate() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if err := g.rotateWithRepo(); err != nil {
		return err
	}
	if g.log != nil {
		g.log.Info("gate key rotated", zap.String("new_key", g.key))
	}
	return nil
}

// SetKey sets a custom key with an optional max-uses limit (0 = unlimited).
func (g *Gate) SetKey(key string, maxUses int) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.repo != nil {
		rec, err := g.repo.InsertToken(key, maxUses)
		if err != nil {
			return fmt.Errorf("set key: insert token: %w", err)
		}
		g.keyID = rec.ID
	}
	g.key = key
	g.maxUses = maxUses
	g.useCount = 0
	if g.log != nil {
		g.log.Info("gate key set to custom value", zap.String("key", key))
	}
	return nil
}

// SetMaxUses updates the max-uses limit for the current active key (0 = unlimited).
func (g *Gate) SetMaxUses(maxUses int) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.repo != nil {
		if err := g.repo.UpdateMaxUses(g.keyID, maxUses); err != nil {
			return fmt.Errorf("set max uses: %w", err)
		}
	}
	g.maxUses = maxUses
	return nil
}

// RefreshFromDB reloads the active token from the DB into the cache.
// No-op in in-memory mode.
func (g *Gate) RefreshFromDB() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.repo == nil {
		return nil
	}
	active, err := g.repo.GetActiveToken()
	if err != nil {
		return fmt.Errorf("refresh from db: %w", err)
	}
	if active == nil {
		return nil
	}
	g.key = active.Token
	g.keyID = active.ID
	g.useCount = active.UseCount
	g.maxUses = active.MaxUses
	return nil
}

// rotateWithRepo generates a new key and persists it if repo is set.
// Must be called with mu held.
func (g *Gate) rotateWithRepo() error {
	newKey, err := generateSecureKey(32)
	if err != nil {
		return err
	}
	if g.repo != nil {
		rec, err := g.repo.InsertToken(newKey, g.maxUses)
		if err != nil {
			return fmt.Errorf("rotate: insert token: %w", err)
		}
		g.keyID = rec.ID
	}
	g.key = newKey
	g.useCount = 0
	return nil
}

// rotate is the in-memory-only rotation used by New() before a repo is set.
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

// Close releases the underlying repository's database connection and reverts
// the gate to in-memory mode. Safe to call if no repo is set.
func (g *Gate) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.repo == nil {
		return nil
	}
	err := g.repo.Close()
	g.repo = nil
	return err
}

// generateSecureKey creates a cryptographically random hex string.
func generateSecureKey(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random key: %w", err)
	}
	return hex.EncodeToString(b), nil
}
