package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const redisKeyPrefix = "session:"

// redisSessionStore is a SessionStore backed by Redis.
// Sessions survive pod restarts and are shared across replicas.
type redisSessionStore struct {
	client *redis.Client
}

// NewRedisSessionStore creates a new Redis-backed session store.
// The redisURL should include credentials and DB if needed
// (e.g. "redis://:password@redis:6379/0").
func NewRedisSessionStore(redisURL string) (SessionStore, error) {
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

	return &redisSessionStore{client: client}, nil
}

func (r *redisSessionStore) Save(ctx context.Context, id string, data sessionData, ttl time.Duration) error {
	payload, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal session data: %w", err)
	}

	return r.client.Set(ctx, redisKeyPrefix+id, payload, ttl).Err()
}

func (r *redisSessionStore) Load(ctx context.Context, id string) (*sessionData, error) {
	val, err := r.client.Get(ctx, redisKeyPrefix+id).Result()
	if err == redis.Nil {
		return nil, fmt.Errorf("session not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to load session from Redis: %w", err)
	}

	var data sessionData
	if err := json.Unmarshal([]byte(val), &data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session data: %w", err)
	}

	return &data, nil
}

func (r *redisSessionStore) Delete(ctx context.Context, id string) error {
	return r.client.Del(ctx, redisKeyPrefix+id).Err()
}
