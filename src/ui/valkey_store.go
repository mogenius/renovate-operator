package ui

import (
	"context"
	"crypto/cipher"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/valkey-io/valkey-go"
)

const valkeyKeyPrefix = "session:"

// valkeySessionStore is a SessionStore backed by Valkey.
// Sessions survive pod restarts and are shared across replicas.
// Session data is encrypted at rest using AES-GCM.
type valkeySessionStore struct {
	client valkey.Client
	gcm    cipher.AEAD
}

// NewValkeySessionStore creates a new Valkey-backed session store.
// The valkeyURL should include credentials and DB if needed
// (e.g. "redis://:password@valkey:6379/0").
// The encryptionKey is used to encrypt session data at rest.
func NewValkeySessionStore(valkeyURL string, encryptionKey [32]byte) (SessionStore, error) {
	opts, err := valkey.ParseURL(valkeyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid Valkey URL: %w", err)
	}
	// Disable client-side caching — we don't need it for session storage
	// and it requires RESP3 + CLIENT TRACKING support on the server.
	opts.DisableCache = true

	client, err := valkey.NewClient(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create Valkey client: %w", err)
	}

	// Verify connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Do(ctx, client.B().Ping().Build()).Error(); err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to connect to Valkey: %w", err)
	}

	gcm, err := newGCM(encryptionKey)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("failed to create GCM for Valkey store: %w", err)
	}

	return &valkeySessionStore{client: client, gcm: gcm}, nil
}

func (v *valkeySessionStore) Save(ctx context.Context, id string, data sessionData, ttl time.Duration) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal session data: %w", err)
	}

	sealed, err := sealGCM(v.gcm, payload)
	if err != nil {
		return fmt.Errorf("failed to encrypt session data: %w", err)
	}
	encoded := base64.StdEncoding.EncodeToString(sealed)

	return v.client.Do(ctx, v.client.B().Set().Key(valkeyKeyPrefix+id).Value(encoded).Ex(ttl).Build()).Error()
}

func (v *valkeySessionStore) Load(ctx context.Context, id string) (*sessionData, error) {
	val, err := v.client.Do(ctx, v.client.B().Get().Key(valkeyKeyPrefix+id).Build()).ToString()
	if valkey.IsValkeyNil(err) {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load session from Valkey: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(val)
	if err != nil {
		return nil, fmt.Errorf("failed to decode session data: %w", err)
	}

	plaintext, err := openGCM(v.gcm, ciphertext)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt session data: %w", err)
	}

	var data sessionData
	if err := json.Unmarshal(plaintext, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session data: %w", err)
	}

	return &data, nil
}

func (v *valkeySessionStore) Delete(ctx context.Context, id string) error {
	return v.client.Do(ctx, v.client.B().Del().Key(valkeyKeyPrefix+id).Build()).Error()
}

func (v *valkeySessionStore) Close() error {
	v.client.Close()
	return nil
}
