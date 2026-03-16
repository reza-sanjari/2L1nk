package session

import (
	"2L1nk/internal/models"
	"crypto/ed25519"
	"fmt"
	"sync"
	"testing"
)

func newTestUser(sessionID, username string) *User {
	pub, _, err := ed25519.GenerateKey(nil)
	if err != nil {
		panic(err)
	}

	return &User{
		SessionID:            sessionID,
		PublicKey:            pub,
		PublicKeyFingerprint: "fp-" + sessionID,
		Username:             username,
		Mode:                 models.UserMode(0),
	}
}

func TestStoreAddAndGet(t *testing.T) {
	store := NewStore()
	user := newTestUser("session-1", "alice")

	store.Add(user)

	got, ok := store.Get("session-1")
	if !ok {
		t.Fatal("expected session to exist")
	}
	if got == nil {
		t.Fatal("expected user, got nil")
	}
	if got.SessionID != user.SessionID {
		t.Fatalf("expected SessionID %q, got %q", user.SessionID, got.SessionID)
	}
	if got.Username != user.Username {
		t.Fatalf("expected Username %q, got %q", user.Username, got.Username)
	}
	if !store.UsernameExists("alice") {
		t.Fatal("expected username to be marked as existing")
	}
}

func TestStoreGet_UnknownSession(t *testing.T) {
	store := NewStore()

	got, ok := store.Get("does-not-exist")
	if ok {
		t.Fatal("expected missing session to return ok=false")
	}
	if got != nil {
		t.Fatalf("expected nil user for missing session, got %+v", got)
	}
}

func TestStoreRemove_RemovesSessionAndUsername(t *testing.T) {
	store := NewStore()
	user := newTestUser("session-1", "alice")

	store.Add(user)
	store.Remove("session-1")

	got, ok := store.Get("session-1")
	if ok {
		t.Fatal("expected session to be removed")
	}
	if got != nil {
		t.Fatal("expected removed session to return nil user")
	}
	if store.UsernameExists("alice") {
		t.Fatal("expected username to be removed as well")
	}
}

func TestStoreRemove_UnknownSessionDoesNotPanic(t *testing.T) {
	store := NewStore()

	store.Remove("unknown-session")

	// pass if no panic and store still behaves normally
	if _, ok := store.Get("unknown-session"); ok {
		t.Fatal("expected unknown session to still not exist")
	}
}

func TestStoreConcurrentAccess_NoRaceOrPanic(t *testing.T) {
	store := NewStore()

	const n = 100
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)

		go func(i int) {
			defer wg.Done()

			sessionID := fmt.Sprintf("session-%d", i)
			username := fmt.Sprintf("user-%d", i)

			user := newTestUser(sessionID, username)

			store.Add(user)

			got, ok := store.Get(sessionID)
			if !ok || got == nil {
				t.Errorf("expected to get session %q", sessionID)
				return
			}

			if got.Username != username {
				t.Errorf("expected username %q, got %q", username, got.Username)
			}

			store.Remove(sessionID)

			if _, ok := store.Get(sessionID); ok {
				t.Errorf("expected session %q to be removed", sessionID)
			}
		}(i)
	}

	wg.Wait()
}
