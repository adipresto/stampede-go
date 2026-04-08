package stampede

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

// Fetcher is a function type that retrieves entities from the database.
// It should return a map of ID to Entity.
type Fetcher[K comparable, T any] func(ctx context.Context, ids []K) (map[K]T, error)

// Registry manages the shared resources for multiple entities.
type Registry struct {
	redis *redis.Client
}

// NewRegistry creates a new central registry for stampede entities.
func NewRegistry(redis *redis.Client) *Registry {
	return &Registry{
		redis: redis,
	}
}

// Entity manages the caching logic for a specific type of entity.
type Entity[K comparable, T any] struct {
	registry *Registry
	group    *singleflight.Group
	fetch    Fetcher[K, T]
	config   Config
	mu       sync.RWMutex
}

// Register declaratively defines a new entity cache within a registry.
func Register[K comparable, T any](r *Registry, prefix string, fetch Fetcher[K, T], opts ...Option) *Entity[K, T] {
	cfg := DefaultConfig(prefix)
	for _, opt := range opts {
		opt(&cfg)
	}

	return &Entity[K, T]{
		registry: r,
		group:    &singleflight.Group{},
		fetch:    fetch,
		config:   cfg,
	}
}

// cacheKey returns the formatted redis key for a given ID.
func (e *Entity[K, T]) cacheKey(id K) string {
	return fmt.Sprintf("%s:id:%v", e.config.Prefix, id)
}

// Get retrieves a single entity by its ID.
// It implements the Cache-Aside pattern with Singleflight protection.
func (e *Entity[K, T]) Get(ctx context.Context, id K) (T, error) {
	var val T
	key := e.cacheKey(id)

	// 1. Try Cache
	data, err := e.registry.redis.Get(ctx, key).Bytes()
	if err == nil {
		err = json.Unmarshal(data, &val)
		return val, err
	}

	// 2. Singleflight protection for Cache Miss
	result, err, _ := e.group.Do(fmt.Sprintf("get:%v", id), func() (interface{}, error) {
		// Double check cache inside singleflight
		data, err := e.registry.redis.Get(ctx, key).Bytes()
		if err == nil {
			var v T
			if err := json.Unmarshal(data, &v); err == nil {
				return v, nil
			}
		}

		// Fetch from DB (using the batch fetcher with a single ID)
		results, err := e.fetch(ctx, []K{id})
		if err != nil {
			return nil, err
		}

		v, ok := results[id]
		if !ok {
			return nil, fmt.Errorf("entity not found: %v", id)
		}

		// Save to cache
		payload, _ := json.Marshal(v)
		e.registry.redis.Set(ctx, key, payload, e.config.TTL)

		return v, nil
	})

	if err != nil {
		return val, err
	}

	return result.(T), nil
}

// MGet retrieves multiple entities by their IDs.
// It implements the "ID Projection" auto-repair logic.
func (e *Entity[K, T]) MGet(ctx context.Context, ids []K) ([]T, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = e.cacheKey(id)
	}

	// 1. Multiple Get from Redis
	results := make([]T, len(ids))
	redisResults, err := e.registry.redis.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}

	missingIDs := make([]K, 0)
	missingIndices := make([]int, 0)

	for i, res := range redisResults {
		if res == nil {
			missingIDs = append(missingIDs, ids[i])
			missingIndices = append(missingIndices, i)
			continue
		}

		// res is a string here from go-redis MGet
		err := json.Unmarshal([]byte(res.(string)), &results[i])
		if err != nil {
			missingIDs = append(missingIDs, ids[i])
			missingIndices = append(missingIndices, i)
		}
	}

	// 2. Auto-Repair: Fetch missing IDs from DB in a single batch
	if len(missingIDs) > 0 {
		dbResults, err := e.fetch(ctx, missingIDs)
		if err != nil {
			return nil, err
		}

		pipe := e.registry.redis.Pipeline()
		for i, originalIdx := range missingIndices {
			id := missingIDs[i]
			if val, ok := dbResults[id]; ok {
				results[originalIdx] = val
				payload, _ := json.Marshal(val)
				pipe.Set(ctx, e.cacheKey(id), payload, e.config.TTL)
			}
		}
		_, _ = pipe.Exec(ctx)
	}

	return results, nil
}
