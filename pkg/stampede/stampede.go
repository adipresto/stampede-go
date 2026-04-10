package stampede

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"time"
	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

// Fetcher is a function type that retrieves entities from the database.
// It should return a map of ID to Entity.
type Fetcher[K comparable, T any] func(ctx context.Context, ids []K) (map[K]T, error)

type batchRequest[K comparable, T any] struct {
	ids  []K
	resp chan map[K]T
	err  chan error
}

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
	batchChan chan batchRequest[K, T]
}

// Register declaratively defines a new entity cache within a registry.
func Register[K comparable, T any](r *Registry, prefix string, fetch Fetcher[K, T], opts ...Option) *Entity[K, T] {
	cfg := DefaultConfig(prefix)
	for _, opt := range opts {
		opt(&cfg)
	}

	e := &Entity[K, T]{
		registry:  r,
		group:     &singleflight.Group{},
		fetch:     fetch,
		config:    cfg,
		batchChan: make(chan batchRequest[K, T], 100),
	}

	go e.batchingLoop()

	return e
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

	// 2. Auto-Repair: Use Global Batcher for missing IDs
	if len(missingIDs) > 0 {
		resp := make(chan map[K]T, 1)
		errChan := make(chan error, 1)
		e.batchChan <- batchRequest[K, T]{
			ids:  missingIDs,
			resp: resp,
			err:  errChan,
		}

		var dbResults map[K]T
		select {
		case res := <-resp:
			dbResults = res
		case err := <-errChan:
			return nil, err
		case <-ctx.Done():
			return nil, ctx.Err()
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

func (e *Entity[K, T]) batchingLoop() {
	for request := range e.batchChan {
		// Start a new batch
		ids := make(map[K]struct{})
		requests := []batchRequest[K, T]{request}

		for _, id := range request.ids {
			ids[id] = struct{}{}
		}

		// Fill window
		timer := time.NewTimer(e.config.BatchWait)
		stop := false

		for !stop {
			select {
			case req := <-e.batchChan:
				requests = append(requests, req)
				for _, id := range req.ids {
					ids[id] = struct{}{}
				}
				if len(ids) >= e.config.MaxBatchSize {
					stop = true
				}
			case <-timer.C:
				stop = true
			}
		}

		// Execute Batch Fetch
		uniqueIDs := make([]K, 0, len(ids))
		for id := range ids {
			uniqueIDs = append(uniqueIDs, id)
		}

		dbResults, err := e.fetch(context.Background(), uniqueIDs)

		// Distribute results
		for _, req := range requests {
			if err != nil {
				req.err <- err
				continue
			}

			// Extract only IDs requested by this specific caller
			subset := make(map[K]T)
			for _, id := range req.ids {
				if val, ok := dbResults[id]; ok {
					subset[id] = val
				}
			}
			req.resp <- subset
		}
	}
}

// GetFields retrieves a single entity and returns only selected fields.
// Projection happens in memory after fetching the full entity from cache/DB.
func (e *Entity[K, T]) GetFields(ctx context.Context, id K, fields []string) (map[string]any, error) {
	val, err := e.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	return e.projectFields(val, fields), nil
}

// MGetFields retrieves multiple entities and returns only selected fields for each.
func (e *Entity[K, T]) MGetFields(ctx context.Context, ids []K, fields []string) ([]map[string]any, error) {
	vals, err := e.MGet(ctx, ids)
	if err != nil {
		return nil, err
	}

	results := make([]map[string]any, len(vals))
	for i, v := range vals {
		results[i] = e.projectFields(v, fields)
	}
	return results, nil
}

// projectFields is a helper that converts an entity to a map and filters keys.
func (e *Entity[K, T]) projectFields(val T, fields []string) map[string]any {
	// 1. Convert struct/type to map using JSON (respects JSON tags)
	payload, _ := json.Marshal(val)
	var fullMap map[string]any
	json.Unmarshal(payload, &fullMap)

	// 2. If no fields specified, return everything
	if len(fields) == 0 {
		return fullMap
	}

	// 3. Filter only requested fields
	filtered := make(map[string]any)
	for _, f := range fields {
		if v, ok := fullMap[f]; ok {
			filtered[f] = v
		}
	}
	return filtered
}
