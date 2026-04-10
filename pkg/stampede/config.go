package stampede

import "time"

// Config holds the configuration for a stampede entity.
type Config struct {
	// TTL is the time-to-live for the entity in the cache.
	TTL time.Duration
	// Prefix is the internal prefix used for the keys in Redis.
	// Format: prefix:id:{ID}
	Prefix string
	// BatchWait is the duration to gather IDs into a global batch (default: 5ms).
	BatchWait time.Duration
	// MaxBatchSize is the maximum number of IDs before triggering a fetch (default: 100).
	MaxBatchSize int
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig(prefix string) Config {
	return Config{
		TTL:          1 * time.Hour,
		Prefix:       prefix,
		BatchWait:    5 * time.Millisecond,
		MaxBatchSize: 100,
	}
}

// Option is a function that modifies a Config.
type Option func(*Config)

// WithTTL sets the TTL for the entity.
func WithTTL(ttl time.Duration) Option {
	return func(c *Config) {
		c.TTL = ttl
	}
}

// WithBatching configures the global batching parameters.
func WithBatching(wait time.Duration, maxSize int) Option {
	return func(c *Config) {
		c.BatchWait = wait
		c.MaxBatchSize = maxSize
	}
}
