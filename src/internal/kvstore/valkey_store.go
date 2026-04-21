package kvstore

import (
	"context"
	"encoding/base64"
	"fmt"
	"time"

	"github.com/valkey-io/valkey-go"
)

// valkeyKVStore is a KVStore backed by Valkey.
// It stores raw bytes (base64-encoded for wire safety) with no
// encryption or key prefixing — those concerns belong to consumers.
type valkeyKVStore struct {
	client valkey.Client
}

// NewValkeyKVStore creates a new Valkey-backed KVStore.
// The valkeyURL should include credentials and DB if needed
// (e.g. "redis://:password@valkey:6379/0").
func NewValkeyKVStore(valkeyURL string) (KVStore, error) {
	opts, err := valkey.ParseURL(valkeyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid Valkey URL: %w", err)
	}
	// Disable client-side caching — we don't need it for KV storage
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

	return &valkeyKVStore{client: client}, nil
}

func (v *valkeyKVStore) Put(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	encoded := base64.StdEncoding.EncodeToString(value)
	return v.client.Do(ctx, v.client.B().Set().Key(key).Value(encoded).Ex(ttl).Build()).Error()
}

func (v *valkeyKVStore) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := v.client.Do(ctx, v.client.B().Get().Key(key).Build()).ToString()
	if valkey.IsValkeyNil(err) {
		return nil, ErrKeyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get key from Valkey: %w", err)
	}

	decoded, err := base64.StdEncoding.DecodeString(val)
	if err != nil {
		return nil, fmt.Errorf("failed to decode value: %w", err)
	}

	return decoded, nil
}

func (v *valkeyKVStore) Del(ctx context.Context, key string) error {
	return v.client.Do(ctx, v.client.B().Del().Key(key).Build()).Error()
}

func (v *valkeyKVStore) Close() error {
	v.client.Close()
	return nil
}
