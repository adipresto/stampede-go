package stampede

import "time"

// Config holds the configuration for a stampede entity.
type Config struct {
	// TTL is the time-to-live for the entity in the cache.
	TTL time.Duration
	// Prefix is the internal prefix used for the keys in Redis.
	// Format: prefix:id:{ID}
	Prefix string
}

// DefaultConfig returns a configuration with sensible defaults.
func DefaultConfig(prefix string) Config {
	return Config{
		TTL:    1 * time.Hour,
		Prefix: prefix,
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
