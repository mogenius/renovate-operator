package ui

import (
	"context"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/redis/go-redis/v9"
)

const redisKeyPrefix = "session:"

// redisSessionStore is a SessionStore backed by Redis.
// Sessions survive pod restarts and are shared across replicas.
// Session data is encrypted at rest using AES-GCM.
type redisSessionStore struct {
	client *redis.Client
	gcm    cipher.AEAD
}

// NewRedisSessionStore creates a new Redis-backed session store.
// The redisURL should include credentials and DB if needed
// (e.g. "redis://:password@redis:6379/0").
// The encryptionKey is used to encrypt session data at rest.
func NewRedisSessionStore(redisURL string, encryptionKey [32]byte) (SessionStore, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("invalid Redis URL: %w", err)
	}

	client := redis.NewClient(opts)

	// Verify connectivity
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	gcm, err := newGCM(encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM for Redis store: %w", err)
	}

	return &redisSessionStore{client: client, gcm: gcm}, nil
}

func (r *redisSessionStore) Save(ctx context.Context, id string, data sessionData, ttl time.Duration) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal session data: %w", err)
	}

	nonce := make([]byte, r.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return fmt.Errorf("failed to generate nonce: %w", err)
	}
	encrypted := r.gcm.Seal(nonce, nonce, payload, nil)
	encoded := base64.StdEncoding.EncodeToString(encrypted)

	return r.client.Set(ctx, redisKeyPrefix+id, encoded, ttl).Err()
}

func (r *redisSessionStore) Load(ctx context.Context, id string) (*sessionData, error) {
	val, err := r.client.Get(ctx, redisKeyPrefix+id).Result()
	if err == redis.Nil {
		return nil, ErrSessionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load session from Redis: %w", err)
	}

	ciphertext, err := base64.StdEncoding.DecodeString(val)
	if err != nil {
		return nil, fmt.Errorf("failed to decode session data: %w", err)
	}

	nonceSize := r.gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("session ciphertext too short")
	}
	nonce, cipherBytes := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := r.gcm.Open(nil, nonce, cipherBytes, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt session data: %w", err)
	}

	var data sessionData
	if err := json.Unmarshal(plaintext, &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session data: %w", err)
	}

	return &data, nil
}

func (r *redisSessionStore) Delete(ctx context.Context, id string) error {
	return r.client.Del(ctx, redisKeyPrefix+id).Err()
}
