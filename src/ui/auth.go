package ui

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-logr/logr"
)

const (
	sessionCookieName   = "renovate_session"
	stateCookieName     = "oauth_state"
	authCompletedCookie = "auth_completed"
	sessionDuration     = 24 * time.Hour
)

type contextKey string

const sessionContextKey contextKey = "session"

type sessionData struct {
	Email       string   `json:"email"`
	Name        string   `json:"name"`
	Expiry      int64    `json:"exp"`
	AccessToken string   `json:"at,omitempty"`
	Groups      []string `json:"groups"`
}

// getSessionFromContext retrieves the session from the request context.
func getSessionFromContext(r *http.Request) *sessionData {
	session, ok := r.Context().Value(sessionContextKey).(*sessionData)
	if !ok {
		return nil
	}
	return session
}

// AuthProvider is the interface that both OIDC and GitHub OAuth implement.
type AuthProvider interface {
	HandleLogin(w http.ResponseWriter, r *http.Request)
	HandleCallback(w http.ResponseWriter, r *http.Request)
	HandleComplete(w http.ResponseWriter, r *http.Request)
	HandleLogout(w http.ResponseWriter, r *http.Request)
	HandleAuthStatus(w http.ResponseWriter, r *http.Request)
	AuthMiddleware(next http.Handler) http.Handler
	SupportsGroups() bool
}

// baseAuth contains shared session and middleware logic for all auth providers.
type baseAuth struct {
	encryptionKey [32]byte
	gcm           cipher.AEAD
	logger        logr.Logger
	sessionStore  SessionStore
}

// ComputeEncryptionKey derives a 32-byte AES key from a session secret.
// If secret is empty, a cryptographically random key is generated.
func ComputeEncryptionKey(secret string) ([32]byte, error) {
	var key [32]byte
	if secret != "" {
		key = sha256.Sum256([]byte(secret))
	} else {
		if _, err := io.ReadFull(rand.Reader, key[:]); err != nil {
			return key, fmt.Errorf("failed to generate encryption key: %w", err)
		}
	}
	return key, nil
}

// newBaseAuth initialises the shared auth fields including the pre-computed
// AES-GCM cipher derived from sessionSecret.
func newBaseAuth(sessionSecret string, logger logr.Logger, store SessionStore) (baseAuth, error) {
	key, err := ComputeEncryptionKey(sessionSecret)
	if err != nil {
		return baseAuth{}, err
	}

	gcm, err := newGCM(key)
	if err != nil {
		return baseAuth{}, err
	}

	return baseAuth{
		encryptionKey: key,
		gcm:           gcm,
		logger:        logger,
		sessionStore:  store,
	}, nil
}

// newGCM creates an AES-GCM cipher from the given 32-byte key.
func newGCM(key [32]byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	return gcm, nil
}

func isPublicPath(path string) bool {
	// Health, auth endpoints, auth status API
	if path == "/health" || path == "/api/v1/auth/status" {
		return true
	}
	if strings.HasPrefix(path, "/auth/") {
		return true
	}
	// Static assets must be accessible without auth so the page can render
	if strings.HasPrefix(path, "/js/") || strings.HasPrefix(path, "/css/") ||
		strings.HasPrefix(path, "/assets/") || path == "/favicon.ico" {
		return true
	}
	return false
}

func (b *baseAuth) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Allow public paths without authentication
		if isPublicPath(path) {
			next.ServeHTTP(w, r)
			return
		}

		// Check session
		session, err := b.getSession(r)
		if err != nil || session == nil {
			if err != nil {
				b.logger.Info("session check failed", "path", path, "error", err.Error())
			}

			// Detect auth redirect loop: if the auth_completed marker cookie is
			// present, the OAuth callback just succeeded but the session cookie
			// cannot be read. This typically means the encryption key differs
			// between replicas (SESSION_SECRET not set).
			if _, markerErr := r.Cookie(authCompletedCookie); markerErr == nil {
				http.SetCookie(w, &http.Cookie{Name: authCompletedCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
				w.Header().Set("Content-Type", "text/plain")
				w.WriteHeader(http.StatusInternalServerError)
				_, err := fmt.Fprintf(w, "Authentication loop detected: login succeeded but the session cookie "+
					"could not be verified. This usually means GITHUB_SESSION_SECRET or OIDC_SESSION_SECRET "+
					"is not set, or differs between replicas. It can also occur when the session store "+
					"is unavailable (pod restart with in-memory store, or Redis connectivity issue).")
				if err != nil {
					b.logger.Error(err, "failed to write error response")
				}
				return
			}
			// Audit log failed authentication attempt
			b.logger.V(1).Info("Unauthenticated request rejected",
				"path", path,
				"method", r.Method,
				"remote_addr", r.RemoteAddr,
				"user_agent", r.UserAgent())

			// API requests get 401
			if strings.HasPrefix(path, "/api/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}
			// UI requests get redirected to login
			b.logger.Info("redirecting to login", "path", path)
			http.Redirect(w, r, "/auth/login", http.StatusFound)
			return
		}

		// Clean up marker cookie after successful session read
		if _, markerErr := r.Cookie(authCompletedCookie); markerErr == nil {
			http.SetCookie(w, &http.Cookie{Name: authCompletedCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
		}

		// Add session to context
		ctx := context.WithValue(r.Context(), sessionContextKey, session)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (b *baseAuth) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	session, _ := b.getSession(r)

	result := map[string]any{
		"enabled": true,
	}

	if session != nil {
		result["authenticated"] = true
		result["email"] = session.Email
		result["name"] = session.Name
	} else {
		result["authenticated"] = false
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func isHTTPS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return r.Header.Get("X-Forwarded-Proto") == "https"
}

// buildCompleteURL encrypts the session data and returns a redirect URL to
// /auth/complete. This separates the OAuth callback (which some proxies
// interfere with) from the cookie-setting step.
func (b *baseAuth) buildCompleteURL(ctx context.Context, email, name string, opts ...func(*sessionData)) (string, error) {
	session := sessionData{
		Email:  email,
		Name:   name,
		Groups: []string{},
		Expiry: time.Now().Add(sessionDuration).Unix(),
	}
	for _, opt := range opts {
		opt(&session)
	}

	encrypted, err := b.encryptSession(ctx, session)
	if err != nil {
		return "", err
	}

	return "/auth/complete?s=" + url.QueryEscape(encrypted), nil
}

// handleComplete reads the encrypted session token from the URL query parameter,
// validates it, sets the session cookie, and redirects to /.
// This handler is called on a separate first-party request (not the OAuth
// callback), so reverse proxies won't strip the Set-Cookie header.
func (b *baseAuth) handleComplete(w http.ResponseWriter, r *http.Request) {
	sessionValue := r.URL.Query().Get("s")
	if sessionValue == "" {
		http.Error(w, "missing session parameter", http.StatusBadRequest)
		return
	}

	// Validate the encrypted token can be decrypted
	session, err := b.decryptSession(r.Context(), sessionValue)
	if err != nil {
		b.logger.Error(err, "failed to decrypt session token in /auth/complete")
		http.Error(w, "invalid session token", http.StatusBadRequest)
		return
	}

	secure := isHTTPS(r)
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    sessionValue,
		Path:     "/",
		MaxAge:   int(sessionDuration.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     authCompletedCookie,
		Value:    "1",
		Path:     "/",
		MaxAge:   60,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})

	b.logger.Info("session cookie set via /auth/complete", "email", session.Email, "name", session.Name, "secure", secure, "cookieLen", len(sessionValue))

	err = writeCallbackRedirect(w, "/")
	if err != nil {
		b.logger.Error(err, "failed to write callback redirect response")
	}
}

// deleteSession removes the session from the server-side store.
func (b *baseAuth) deleteSession(r *http.Request) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return
	}
	id, err := b.decryptString(cookie.Value)
	if err != nil {
		return
	}
	if err := b.sessionStore.Delete(r.Context(), id); err != nil {
		b.logger.V(1).Info("failed to delete session from store", "error", err)
	}
}

func (b *baseAuth) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
}

func (b *baseAuth) setStateCookie(w http.ResponseWriter, r *http.Request) (string, error) {
	state, err := generateRandomString(32)
	if err != nil {
		return "", err
	}
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    state,
		Path:     "/",
		MaxAge:   300,
		HttpOnly: true,
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode,
	})
	return state, nil
}

func (b *baseAuth) validateStateCookie(r *http.Request) error {
	stateCookie, err := r.Cookie(stateCookieName)
	if err != nil {
		return fmt.Errorf("missing state cookie")
	}
	if r.URL.Query().Get("state") != stateCookie.Value {
		return fmt.Errorf("invalid state")
	}
	return nil
}

func (b *baseAuth) clearStateCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     stateCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
}

func (b *baseAuth) getSession(r *http.Request) (*sessionData, error) {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return nil, err
	}

	session, err := b.decryptSession(r.Context(), cookie.Value)
	if err != nil {
		return nil, err
	}

	// The session store handles TTL-based expiry. Check the in-session expiry
	// field as a secondary safeguard.
	if time.Now().Unix() > session.Expiry {
		return nil, fmt.Errorf("session expired")
	}

	return session, nil
}

func (b *baseAuth) encryptSession(ctx context.Context, session sessionData) (string, error) {
	// Generate a random session ID
	sessionID, err := generateRandomString(32)
	if err != nil {
		return "", fmt.Errorf("failed to generate session ID: %w", err)
	}

	// Store the full session data server-side
	ttl := time.Duration(session.Expiry-time.Now().Unix()) * time.Second
	if ttl <= 0 {
		ttl = sessionDuration
	}
	if err := b.sessionStore.Save(ctx, sessionID, session, ttl); err != nil {
		return "", fmt.Errorf("failed to save session: %w", err)
	}

	// AES-GCM encrypt only the session ID (not the full session data)
	return b.encryptString(sessionID)
}

// encryptString encrypts a plaintext string using AES-GCM and returns a base64url-encoded ciphertext.
func (b *baseAuth) encryptString(plaintext string) (string, error) {
	nonce := make([]byte, b.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := b.gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.URLEncoding.EncodeToString(ciphertext), nil
}

func (b *baseAuth) decryptSession(ctx context.Context, encrypted string) (*sessionData, error) {
	// Decrypt to get the session ID
	sessionID, err := b.decryptString(encrypted)
	if err != nil {
		return nil, err
	}

	// Load full session data from the server-side store
	session, err := b.sessionStore.Load(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("session store load failed: %w", err)
	}

	return session, nil
}

// decryptString decrypts a base64url-encoded AES-GCM ciphertext and returns the plaintext string.
func (b *baseAuth) decryptString(encrypted string) (string, error) {
	data, err := base64.URLEncoding.DecodeString(encrypted)
	if err != nil {
		return "", err
	}

	nonceSize := b.gcm.NonceSize()
	if len(data) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := b.gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}

	return string(plaintext), nil
}

// writeCallbackRedirect sends a 200 response with a meta-refresh redirect.
// This is used instead of http.Redirect (302) in OAuth callbacks because some
// reverse proxies / ingress controllers strip Set-Cookie headers from redirect
// responses, preventing the browser from storing the session cookie.
func writeCallbackRedirect(w http.ResponseWriter, target string) error {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, err := fmt.Fprintf(w, `<!DOCTYPE html><html><head><meta http-equiv="refresh" content="0;url=%s"></head>`+
		`<body>Redirecting...</body></html>`, target)
	return err
}

func generateRandomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
