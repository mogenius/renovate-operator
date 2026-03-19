package ui

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

func TestMemoryStore_SaveAndLoad(t *testing.T) {
	store := NewMemorySessionStore()
	ctx := context.Background()

	session := sessionData{
		Email:  "test@example.com",
		Name:   "Test User",
		Groups: []string{"team-a", "team-b"},
		Expiry: time.Now().Add(1 * time.Hour).Unix(),
	}

	if err := store.Save(ctx, "sid-1", session, 1*time.Hour); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := store.Load(ctx, "sid-1")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.Email != session.Email {
		t.Errorf("Email mismatch: got %s, want %s", loaded.Email, session.Email)
	}
	if loaded.Name != session.Name {
		t.Errorf("Name mismatch: got %s, want %s", loaded.Name, session.Name)
	}
	if len(loaded.Groups) != 2 || loaded.Groups[0] != "team-a" || loaded.Groups[1] != "team-b" {
		t.Errorf("Groups mismatch: got %v", loaded.Groups)
	}
}

func TestMemoryStore_LoadReturnsDeepCopy(t *testing.T) {
	store := NewMemorySessionStore()
	ctx := context.Background()

	session := sessionData{
		Email:  "test@example.com",
		Name:   "Test User",
		Groups: []string{"team-a", "team-b"},
		Expiry: time.Now().Add(1 * time.Hour).Unix(),
	}

	if err := store.Save(ctx, "sid-dc", session, 1*time.Hour); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load and mutate the returned Groups slice
	loaded1, err := store.Load(ctx, "sid-dc")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	loaded1.Groups[0] = "MUTATED"

	// Load again — should get the original data, not the mutated version
	loaded2, err := store.Load(ctx, "sid-dc")
	if err != nil {
		t.Fatalf("Second Load failed: %v", err)
	}
	if loaded2.Groups[0] != "team-a" {
		t.Errorf("Deep copy violated: got %s, want team-a", loaded2.Groups[0])
	}
}

func TestMemoryStore_SaveDeepCopiesInput(t *testing.T) {
	store := NewMemorySessionStore()
	ctx := context.Background()

	groups := []string{"team-a", "team-b"}
	session := sessionData{
		Email:  "test@example.com",
		Name:   "Test User",
		Groups: groups,
		Expiry: time.Now().Add(1 * time.Hour).Unix(),
	}

	if err := store.Save(ctx, "sid-save-dc", session, 1*time.Hour); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Mutate the original slice after saving
	groups[0] = "MUTATED"

	loaded, err := store.Load(ctx, "sid-save-dc")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.Groups[0] != "team-a" {
		t.Errorf("Save did not deep copy: got %s, want team-a", loaded.Groups[0])
	}
}

func TestMemoryStore_Delete(t *testing.T) {
	store := NewMemorySessionStore()
	ctx := context.Background()

	session := sessionData{
		Email:  "test@example.com",
		Name:   "Test User",
		Groups: []string{},
		Expiry: time.Now().Add(1 * time.Hour).Unix(),
	}

	if err := store.Save(ctx, "sid-del", session, 1*time.Hour); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if err := store.Delete(ctx, "sid-del"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := store.Load(ctx, "sid-del")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("Expected ErrSessionNotFound after Delete, got %v", err)
	}
}

func TestMemoryStore_ExpiredEntry(t *testing.T) {
	store := NewMemorySessionStore()
	ctx := context.Background()

	session := sessionData{
		Email:  "test@example.com",
		Name:   "Test User",
		Groups: []string{},
		Expiry: time.Now().Add(-1 * time.Hour).Unix(),
	}

	if err := store.Save(ctx, "sid-exp", session, 1*time.Hour); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Directly set expiresAt to the past instead of relying on time.Sleep
	ms := store.(*memorySessionStore)
	ms.mu.Lock()
	entry := ms.entries["sid-exp"]
	entry.expiresAt = time.Now().Add(-1 * time.Second)
	ms.entries["sid-exp"] = entry
	ms.mu.Unlock()

	_, err := store.Load(ctx, "sid-exp")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("Expected ErrSessionNotFound (expired), got %v", err)
	}
}

func TestMemoryStore_NotFound(t *testing.T) {
	store := NewMemorySessionStore()
	ctx := context.Background()

	_, err := store.Load(ctx, "nonexistent")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("Expected ErrSessionNotFound, got %v", err)
	}
}

func TestCookieSize_WithManyGroups(t *testing.T) {
	key, keyErr := ComputeEncryptionKey("test-secret-key-with-32-chars!!!")
	if keyErr != nil {
		t.Fatalf("ComputeEncryptionKey failed: %v", keyErr)
	}
	base, err := newBaseAuth(key, logr.Discard(), NewMemorySessionStore())
	if err != nil {
		t.Fatalf("Failed to create baseAuth: %v", err)
	}
	auth := &base

	// Generate 100 UUID-like groups (36 chars each)
	groups := make([]string, 100)
	for i := range groups {
		groups[i] = fmt.Sprintf("%08d-1234-5678-9abc-def012345678", i)
	}

	session := sessionData{
		Email:       "user@example.com",
		Name:        "Test User With Many Groups",
		Groups:      groups,
		AccessToken: "gho_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
		Expiry:      time.Now().Add(24 * time.Hour).Unix(),
	}

	ctx := context.Background()
	encrypted, err := auth.encryptSession(ctx, session)
	if err != nil {
		t.Fatalf("encryptSession failed: %v", err)
	}

	// The cookie value should be well under 200 bytes since only the session ID is encrypted
	if len(encrypted) > 200 {
		t.Errorf("Cookie value too large: %d bytes (want < 200). "+
			"Session data should be stored server-side, not in the cookie.", len(encrypted))
	}

	// Verify the full round-trip still works
	decrypted, err := auth.decryptSession(ctx, encrypted)
	if err != nil {
		t.Fatalf("decryptSession failed: %v", err)
	}

	if len(decrypted.Groups) != 100 {
		t.Errorf("Groups count mismatch: got %d, want 100", len(decrypted.Groups))
	}
	if decrypted.Email != session.Email {
		t.Errorf("Email mismatch: got %s, want %s", decrypted.Email, session.Email)
	}
}

func TestRedisStore_Integration(t *testing.T) {
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		t.Skip("REDIS_URL not set, skipping Redis integration test")
	}

	key, keyErr := ComputeEncryptionKey("redis-test-secret")
	if keyErr != nil {
		t.Fatalf("ComputeEncryptionKey failed: %v", keyErr)
	}

	store, err := NewRedisSessionStore(redisURL, key)
	if err != nil {
		t.Fatalf("NewRedisSessionStore failed: %v", err)
	}

	ctx := context.Background()
	session := sessionData{
		Email:  "redis-test@example.com",
		Name:   "Redis Test User",
		Groups: []string{"team-a", "team-b"},
		Expiry: time.Now().Add(1 * time.Hour).Unix(),
	}

	// Save
	if err := store.Save(ctx, "redis-sid-1", session, 1*time.Hour); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load
	loaded, err := store.Load(ctx, "redis-sid-1")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if loaded.Email != session.Email {
		t.Errorf("Email mismatch: got %s, want %s", loaded.Email, session.Email)
	}
	if len(loaded.Groups) != 2 {
		t.Errorf("Groups mismatch: got %v", loaded.Groups)
	}

	// Delete
	if err := store.Delete(ctx, "redis-sid-1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Load after delete
	_, err = store.Load(ctx, "redis-sid-1")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("Expected ErrSessionNotFound after Delete, got %v", err)
	}

	// Load non-existent
	_, err = store.Load(ctx, "redis-nonexistent")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("Expected ErrSessionNotFound for non-existent session, got %v", err)
	}
}
