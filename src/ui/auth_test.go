package ui

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-logr/logr"
)

func TestGetSession_ValidWithGroups(t *testing.T) {
	auth := &baseAuth{
		logger:       logr.Discard(),
		sessionStore: NewMemorySessionStore(),
	}
	key, err := newEncryptionKey("test-secret-key-with-32-chars!!!")
	if err != nil {
		t.Fatalf("Failed to create encryption key: %v", err)
	}
	auth.encryptionKey = key

	// Create a session with groups
	sessionData := sessionData{
		Email:  "test@example.com",
		Name:   "Test User",
		Groups: []string{"team-a"},
		Expiry: time.Now().Add(1 * time.Hour).Unix(),
	}

	encrypted, err := auth.encryptSession(sessionData)
	if err != nil {
		t.Fatalf("Failed to encrypt session: %v", err)
	}

	// Create request with session cookie
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{
		Name:  sessionCookieName,
		Value: encrypted,
	})

	session, err := auth.getSession(req)
	if err != nil {
		t.Fatalf("getSession failed: %v", err)
	}

	if session.Email != "test@example.com" {
		t.Errorf("Expected email test@example.com, got %s", session.Email)
	}
	if len(session.Groups) != 1 || session.Groups[0] != "team-a" {
		t.Errorf("Groups not retrieved correctly: %v", session.Groups)
	}
}

func TestEncryptDecryptSession_PreservesGroups(t *testing.T) {
	auth := &baseAuth{
		logger:       logr.Discard(),
		sessionStore: NewMemorySessionStore(),
	}
	key, err := newEncryptionKey("test-secret-key-with-32-chars!!!")
	if err != nil {
		t.Fatalf("Failed to create encryption key: %v", err)
	}
	auth.encryptionKey = key

	originalSession := sessionData{
		Email:  "test@example.com",
		Name:   "Test User",
		Groups: []string{"team-a", "team-b", "team-c"},
		Expiry: time.Now().Add(1 * time.Hour).Unix(),
	}

	encrypted, err := auth.encryptSession(originalSession)
	if err != nil {
		t.Fatalf("Failed to encrypt session: %v", err)
	}

	decrypted, err := auth.decryptSession(encrypted)
	if err != nil {
		t.Fatalf("Failed to decrypt session: %v", err)
	}

	if len(decrypted.Groups) != len(originalSession.Groups) {
		t.Errorf("Groups length mismatch: expected %d, got %d", len(originalSession.Groups), len(decrypted.Groups))
	}

	for i, group := range originalSession.Groups {
		if decrypted.Groups[i] != group {
			t.Errorf("Group %d mismatch: expected %s, got %s", i, group, decrypted.Groups[i])
		}
	}
}

// TestCookieDeletionFlags verifies proper flags on cookie deletion
func TestCookieDeletionFlags(t *testing.T) {
	auth := &baseAuth{
		logger:       logr.Discard(),
		sessionStore: NewMemorySessionStore(),
	}

	w := httptest.NewRecorder()
	auth.clearSessionCookie(w)

	cookies := w.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("Expected 1 cookie, got %d", len(cookies))
	}

	cookie := cookies[0]
	if cookie.Name != sessionCookieName {
		t.Errorf("Expected cookie name %s, got %s", sessionCookieName, cookie.Name)
	}
	if cookie.MaxAge != -1 {
		t.Errorf("Expected MaxAge -1, got %d", cookie.MaxAge)
	}
	if !cookie.HttpOnly {
		t.Error("Expected HttpOnly to be true")
	}
}

func TestGetSessionFromContext_WithSession(t *testing.T) {
	session := &sessionData{
		Email:  "test@example.com",
		Name:   "Test User",
		Groups: []string{"team-a"},
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	ctx := context.WithValue(req.Context(), sessionContextKey, session)
	req = req.WithContext(ctx)

	retrievedSession := getSessionFromContext(req)
	if retrievedSession == nil {
		t.Fatal("Expected session, got nil")
	}

	if retrievedSession.Email != "test@example.com" {
		t.Errorf("Expected email test@example.com, got %s", retrievedSession.Email)
	}
	if len(retrievedSession.Groups) != 1 || retrievedSession.Groups[0] != "team-a" {
		t.Errorf("Groups not retrieved correctly: %v", retrievedSession.Groups)
	}
}

func TestGetSessionFromContext_NoSession(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)

	retrievedSession := getSessionFromContext(req)
	if retrievedSession != nil {
		t.Errorf("Expected nil session, got %+v", retrievedSession)
	}
}

func TestAuthMiddleware_StoresSessionInContext(t *testing.T) {
	auth := &baseAuth{
		logger:       logr.Discard(),
		sessionStore: NewMemorySessionStore(),
	}
	key, err := newEncryptionKey("test-secret-key-with-32-chars!!!")
	if err != nil {
		t.Fatalf("Failed to create encryption key: %v", err)
	}
	auth.encryptionKey = key

	// Create a valid session cookie
	session := sessionData{
		Email:  "test@example.com",
		Name:   "Test User",
		Groups: []string{"team-a"},
		Expiry: time.Now().Add(1 * time.Hour).Unix(),
	}

	encrypted, err := auth.encryptSession(session)
	if err != nil {
		t.Fatalf("Failed to encrypt session: %v", err)
	}

	// Create a handler that checks if session is in context
	var capturedSession *sessionData
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedSession = getSessionFromContext(r)
		w.WriteHeader(http.StatusOK)
	})

	// Wrap with auth middleware
	middleware := auth.authMiddleware(testHandler)

	// Create request with session cookie
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.AddCookie(&http.Cookie{
		Name:  sessionCookieName,
		Value: encrypted,
	})

	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)

	if capturedSession == nil {
		t.Fatal("Session was not stored in context")
	}

	if capturedSession.Email != "test@example.com" {
		t.Errorf("Expected email test@example.com, got %s", capturedSession.Email)
	}
	if len(capturedSession.Groups) != 1 || capturedSession.Groups[0] != "team-a" {
		t.Errorf("Groups not in context correctly: %v", capturedSession.Groups)
	}
}
