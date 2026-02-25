package ui

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/go-logr/logr"
)

const (
	sessionCookieName = "renovate_session"
	stateCookieName   = "oauth_state"
	sessionDuration   = 24 * time.Hour
)

type sessionData struct {
	Email  string `json:"email"`
	Name   string `json:"name"`
	Expiry int64  `json:"exp"`
}

// AuthProvider is the interface that both OIDC and GitHub OAuth implement.
type AuthProvider interface {
	HandleLogin(w http.ResponseWriter, r *http.Request)
	HandleCallback(w http.ResponseWriter, r *http.Request)
	HandleLogout(w http.ResponseWriter, r *http.Request)
	HandleAuthStatus(w http.ResponseWriter, r *http.Request)
	AuthMiddleware(next http.Handler) http.Handler
}

// baseAuth contains shared session and middleware logic for all auth providers.
type baseAuth struct {
	encryptionKey [32]byte
	logger        logr.Logger
}

func newEncryptionKey(sessionSecret string) ([32]byte, error) {
	var key [32]byte
	if sessionSecret != "" {
		key = sha256.Sum256([]byte(sessionSecret))
	} else {
		if _, err := io.ReadFull(rand.Reader, key[:]); err != nil {
			return key, fmt.Errorf("failed to generate encryption key: %w", err)
		}
	}
	return key, nil
}

func (b *baseAuth) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Always allow health endpoint
		if path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		// Always allow auth endpoints
		if strings.HasPrefix(path, "/auth/") {
			next.ServeHTTP(w, r)
			return
		}

		// Check session
		session, err := b.getSession(r)
		if err != nil || session == nil {
			// API requests get 401
			if strings.HasPrefix(path, "/api/") {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}
			// UI requests get redirected to login
			http.Redirect(w, r, "/auth/login", http.StatusFound)
			return
		}

		next.ServeHTTP(w, r)
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

func (b *baseAuth) setSessionCookie(w http.ResponseWriter, email, name string) error {
	session := sessionData{
		Email:  email,
		Name:   name,
		Expiry: time.Now().Add(sessionDuration).Unix(),
	}

	encrypted, err := b.encryptSession(session)
	if err != nil {
		return err
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    encrypted,
		Path:     "/",
		MaxAge:   int(sessionDuration.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	return nil
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

func (b *baseAuth) setStateCookie(w http.ResponseWriter) (string, error) {
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

	session, err := b.decryptSession(cookie.Value)
	if err != nil {
		return nil, err
	}

	if time.Now().Unix() > session.Expiry {
		return nil, fmt.Errorf("session expired")
	}

	return session, nil
}

func (b *baseAuth) encryptSession(session sessionData) (string, error) {
	plaintext, err := json.Marshal(session)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(b.encryptionKey[:])
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.URLEncoding.EncodeToString(ciphertext), nil
}

func (b *baseAuth) decryptSession(encrypted string) (*sessionData, error) {
	data, err := base64.URLEncoding.DecodeString(encrypted)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(b.encryptionKey[:])
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce, ciphertext := data[:nonceSize], data[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, err
	}

	var session sessionData
	if err := json.Unmarshal(plaintext, &session); err != nil {
		return nil, err
	}

	return &session, nil
}

func generateRandomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(b), nil
}
