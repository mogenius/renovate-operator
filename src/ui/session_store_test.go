package ui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
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

// --- Valkey store tests (using miniredis — wire-compatible) ---

func newTestValkeyStore(t *testing.T) (SessionStore, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	key, err := ComputeEncryptionKey("test-valkey-secret")
	if err != nil {
		t.Fatalf("ComputeEncryptionKey failed: %v", err)
	}
	store, err := NewValkeySessionStore("redis://"+mr.Addr()+"/0", key)
	if err != nil {
		t.Fatalf("NewValkeySessionStore failed: %v", err)
	}
	return store, mr
}

func TestValkeyStore_SaveAndLoad(t *testing.T) {
	store, _ := newTestValkeyStore(t)
	ctx := context.Background()

	session := sessionData{
		Email:  "valkey@example.com",
		Name:   "Valkey User",
		Groups: []string{"team-a", "team-b"},
		Expiry: time.Now().Add(1 * time.Hour).Unix(),
	}

	if err := store.Save(ctx, "rs-1", session, 1*time.Hour); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := store.Load(ctx, "rs-1")
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

func TestValkeyStore_Delete(t *testing.T) {
	store, _ := newTestValkeyStore(t)
	ctx := context.Background()

	session := sessionData{
		Email:  "valkey@example.com",
		Groups: []string{},
		Expiry: time.Now().Add(1 * time.Hour).Unix(),
	}

	if err := store.Save(ctx, "rs-del", session, 1*time.Hour); err != nil {
		t.Fatalf("Save failed: %v", err)
	}
	if err := store.Delete(ctx, "rs-del"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	_, err := store.Load(ctx, "rs-del")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("Expected ErrSessionNotFound after Delete, got %v", err)
	}
}

func TestValkeyStore_NotFound(t *testing.T) {
	store, _ := newTestValkeyStore(t)
	ctx := context.Background()

	_, err := store.Load(ctx, "nonexistent")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("Expected ErrSessionNotFound, got %v", err)
	}
}

func TestValkeyStore_Expiry(t *testing.T) {
	store, mr := newTestValkeyStore(t)
	ctx := context.Background()

	session := sessionData{
		Email:  "valkey@example.com",
		Groups: []string{},
		Expiry: time.Now().Add(10 * time.Second).Unix(),
	}

	if err := store.Save(ctx, "rs-exp", session, 10*time.Second); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify it exists before expiry
	if _, err := store.Load(ctx, "rs-exp"); err != nil {
		t.Fatalf("Load before expiry failed: %v", err)
	}

	// Fast-forward miniredis clock past the TTL
	mr.FastForward(11 * time.Second)

	_, err := store.Load(ctx, "rs-exp")
	if !errors.Is(err, ErrSessionNotFound) {
		t.Errorf("Expected ErrSessionNotFound after expiry, got %v", err)
	}
}

func TestValkeyStore_EncryptionAtRest(t *testing.T) {
	store, mr := newTestValkeyStore(t)
	ctx := context.Background()

	session := sessionData{
		Email:  "secret@example.com",
		Name:   "Secret User",
		Groups: []string{"admin"},
		Expiry: time.Now().Add(1 * time.Hour).Unix(),
	}

	if err := store.Save(ctx, "rs-enc", session, 1*time.Hour); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Read the raw value from miniredis — it should NOT be plaintext JSON
	raw, err := mr.Get(valkeyKeyPrefix + "rs-enc")
	if err != nil {
		t.Fatalf("Failed to read raw value from miniredis: %v", err)
	}

	// Verify raw value is not valid JSON (i.e., it's encrypted)
	var probe json.RawMessage
	if json.Unmarshal([]byte(raw), &probe) == nil {
		t.Error("Raw Valkey value is valid JSON — session data is NOT encrypted at rest")
	}

	// Verify the email is not in the raw value as plaintext
	if strings.Contains(raw, "secret@example.com") {
		t.Error("Raw Valkey value contains plaintext email — session data is NOT encrypted at rest")
	}
}

// --- Factory and URL builder tests ---

func TestBuildValkeyURL_EmptyHost(t *testing.T) {
	result := buildValkeyURL("", "6379", "")
	if result != "" {
		t.Errorf("Expected empty string for empty host, got %q", result)
	}
}

func TestBuildValkeyURL_HostAndPort(t *testing.T) {
	result := buildValkeyURL("valkey.example.com", "6380", "")
	expected := "redis://valkey.example.com:6380/0"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestBuildValkeyURL_DefaultPort(t *testing.T) {
	result := buildValkeyURL("valkey.example.com", "", "")
	expected := "redis://valkey.example.com:6379/0"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestBuildValkeyURL_WithPassword(t *testing.T) {
	result := buildValkeyURL("valkey.example.com", "6379", "s3cret")
	expected := "redis://:s3cret@valkey.example.com:6379/0"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestBuildValkeyURL_PasswordWithSpecialChars(t *testing.T) {
	result := buildValkeyURL("valkey.example.com", "6379", "p@ss:word/123")
	expected := "redis://:p%40ss%3Aword%2F123@valkey.example.com:6379/0"
	if result != expected {
		t.Errorf("got %q, want %q", result, expected)
	}
}

func TestNewSessionStore_EmptyConfig_ReturnsMemory(t *testing.T) {
	key, err := ComputeEncryptionKey("test-secret")
	if err != nil {
		t.Fatalf("ComputeEncryptionKey failed: %v", err)
	}

	store, storeType, storeErr := NewSessionStore(ValkeyConfig{}, key)
	if storeErr != nil {
		t.Fatalf("NewSessionStore failed: %v", storeErr)
	}
	if storeType != "memory" {
		t.Errorf("Expected store type 'memory', got %q", storeType)
	}
	if store == nil {
		t.Fatal("Expected non-nil store")
	}
}

func TestNewSessionStore_WithValkeyURL_ReturnsValkey(t *testing.T) {
	mr := miniredis.RunT(t)
	key, err := ComputeEncryptionKey("test-secret")
	if err != nil {
		t.Fatalf("ComputeEncryptionKey failed: %v", err)
	}

	store, storeType, storeErr := NewSessionStore(ValkeyConfig{
		URL: "redis://" + mr.Addr() + "/0",
	}, key)
	if storeErr != nil {
		t.Fatalf("NewSessionStore failed: %v", storeErr)
	}
	if storeType != "valkey" {
		t.Errorf("Expected store type 'valkey', got %q", storeType)
	}
	if store == nil {
		t.Fatal("Expected non-nil store")
	}
}

func TestNewSessionStore_WithHost_ReturnsValkey(t *testing.T) {
	mr := miniredis.RunT(t)
	key, err := ComputeEncryptionKey("test-secret")
	if err != nil {
		t.Fatalf("ComputeEncryptionKey failed: %v", err)
	}

	store, storeType, storeErr := NewSessionStore(ValkeyConfig{
		Host: mr.Host(),
		Port: mr.Port(),
	}, key)
	if storeErr != nil {
		t.Fatalf("NewSessionStore failed: %v", storeErr)
	}
	if storeType != "valkey" {
		t.Errorf("Expected store type 'valkey', got %q", storeType)
	}
	if store == nil {
		t.Fatal("Expected non-nil store")
	}
}

func TestNewSessionStore_URLTakesPrecedenceOverHost(t *testing.T) {
	mr := miniredis.RunT(t)
	key, err := ComputeEncryptionKey("test-secret")
	if err != nil {
		t.Fatalf("ComputeEncryptionKey failed: %v", err)
	}

	// URL points to miniredis, Host points to nowhere
	store, storeType, storeErr := NewSessionStore(ValkeyConfig{
		URL:  "redis://" + mr.Addr() + "/0",
		Host: "nonexistent.invalid",
		Port: "9999",
	}, key)
	if storeErr != nil {
		t.Fatalf("NewSessionStore failed: %v", storeErr)
	}
	if storeType != "valkey" {
		t.Errorf("Expected store type 'valkey', got %q", storeType)
	}
	if store == nil {
		t.Fatal("Expected non-nil store")
	}
}
