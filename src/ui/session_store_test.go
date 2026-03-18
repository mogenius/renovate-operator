package ui

import (
	"context"
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
	if err == nil {
		t.Error("Expected error after Delete, got nil")
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

	// Save with a very short TTL that has already expired
	if err := store.Save(ctx, "sid-exp", session, 1*time.Millisecond); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Wait for TTL to pass
	time.Sleep(5 * time.Millisecond)

	_, err := store.Load(ctx, "sid-exp")
	if err == nil {
		t.Error("Expected error for expired session, got nil")
	}
}

func TestMemoryStore_NotFound(t *testing.T) {
	store := NewMemorySessionStore()
	ctx := context.Background()

	_, err := store.Load(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent session, got nil")
	}
}

func TestCookieSize_WithManyGroups(t *testing.T) {
	auth := &baseAuth{
		logger:       logr.Discard(),
		sessionStore: NewMemorySessionStore(),
	}
	key, err := newEncryptionKey("test-secret-key-with-32-chars!!!")
	if err != nil {
		t.Fatalf("Failed to create encryption key: %v", err)
	}
	auth.encryptionKey = key

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

	encrypted, err := auth.encryptSession(session)
	if err != nil {
		t.Fatalf("encryptSession failed: %v", err)
	}

	// The cookie value should be well under 200 bytes since only the session ID is encrypted
	if len(encrypted) > 200 {
		t.Errorf("Cookie value too large: %d bytes (want < 200). "+
			"Session data should be stored server-side, not in the cookie.", len(encrypted))
	}

	// Verify the full round-trip still works
	decrypted, err := auth.decryptSession(encrypted)
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

	store, err := NewRedisSessionStore(redisURL)
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
	if err == nil {
		t.Error("Expected error after Delete, got nil")
	}

	// Load non-existent
	_, err = store.Load(ctx, "redis-nonexistent")
	if err == nil {
		t.Error("Expected error for non-existent session, got nil")
	}
}
