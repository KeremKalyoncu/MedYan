package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// CacheManager provides caching functionality using Redis
type CacheManager struct {
	client *redis.Client
	logger *zap.Logger
	prefix string
}

// NewCacheManager creates a new cache manager
func NewCacheManager(redisAddr string, logger *zap.Logger) (*CacheManager, error) {
	client := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	// Test connection
	if err := client.Ping(context.Background()).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	return &CacheManager{
		client: client,
		logger: logger,
		prefix: "cache:",
	}, nil
}

// Set stores a value in cache with expiration
func (cm *CacheManager) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	fullKey := cm.prefix + key

	// Marshal value to JSON
	data, err := json.Marshal(value)
	if err != nil {
		cm.logger.Error("Failed to marshal cache value",
			zap.String("key", key),
			zap.Error(err),
		)
		return fmt.Errorf("failed to marshal value: %w", err)
	}

	// Store in Redis
	if err := cm.client.Set(ctx, fullKey, data, ttl).Err(); err != nil {
		cm.logger.Error("Failed to set cache",
			zap.String("key", key),
			zap.Error(err),
		)
		return fmt.Errorf("failed to set cache: %w", err)
	}

	cm.logger.Debug("Cache set",
		zap.String("key", key),
		zap.Duration("ttl", ttl),
	)

	return nil
}

// Get retrieves a value from cache
func (cm *CacheManager) Get(ctx context.Context, key string, dest interface{}) error {
	fullKey := cm.prefix + key

	// Get from Redis
	val, err := cm.client.Get(ctx, fullKey).Result()
	if err == redis.Nil {
		cm.logger.Debug("Cache miss", zap.String("key", key))
		return ErrCacheMiss
	}
	if err != nil {
		cm.logger.Error("Failed to get cache",
			zap.String("key", key),
			zap.Error(err),
		)
		return fmt.Errorf("failed to get cache: %w", err)
	}

	// Unmarshal value
	if err := json.Unmarshal([]byte(val), dest); err != nil {
		cm.logger.Error("Failed to unmarshal cache value",
			zap.String("key", key),
			zap.Error(err),
		)
		return fmt.Errorf("failed to unmarshal value: %w", err)
	}

	cm.logger.Debug("Cache hit", zap.String("key", key))
	return nil
}

// Delete removes a value from cache
func (cm *CacheManager) Delete(ctx context.Context, key string) error {
	fullKey := cm.prefix + key

	if err := cm.client.Del(ctx, fullKey).Err(); err != nil {
		cm.logger.Error("Failed to delete cache",
			zap.String("key", key),
			zap.Error(err),
		)
		return fmt.Errorf("failed to delete cache: %w", err)
	}

	cm.logger.Debug("Cache deleted", zap.String("key", key))
	return nil
}

// DeletePattern deletes all keys matching a pattern
func (cm *CacheManager) DeletePattern(ctx context.Context, pattern string) error {
	fullPattern := cm.prefix + pattern

	keys, err := cm.client.Keys(ctx, fullPattern).Result()
	if err != nil {
		return fmt.Errorf("failed to list keys: %w", err)
	}

	if len(keys) == 0 {
		cm.logger.Debug("No keys matched pattern", zap.String("pattern", pattern))
		return nil
	}

	if err := cm.client.Del(ctx, keys...).Err(); err != nil {
		cm.logger.Error("Failed to delete cache pattern",
			zap.String("pattern", pattern),
			zap.Error(err),
		)
		return fmt.Errorf("failed to delete cache pattern: %w", err)
	}

	cm.logger.Debug("Cache pattern deleted",
		zap.String("pattern", pattern),
		zap.Int("count", len(keys)),
	)

	return nil
}

// Clear removes all cached data
func (cm *CacheManager) Clear(ctx context.Context) error {
	if err := cm.client.FlushDB(ctx).Err(); err != nil {
		cm.logger.Error("Failed to clear cache", zap.Error(err))
		return fmt.Errorf("failed to clear cache: %w", err)
	}

	cm.logger.Info("Cache cleared")
	return nil
}

// Exists checks if a key exists in cache
func (cm *CacheManager) Exists(ctx context.Context, key string) (bool, error) {
	fullKey := cm.prefix + key

	exists, err := cm.client.Exists(ctx, fullKey).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check cache existence: %w", err)
	}

	return exists > 0, nil
}

// GetTTL returns the remaining TTL of a key
func (cm *CacheManager) GetTTL(ctx context.Context, key string) (time.Duration, error) {
	fullKey := cm.prefix + key

	ttl, err := cm.client.TTL(ctx, fullKey).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get TTL: %w", err)
	}

	return ttl, nil
}

// SetExpire updates the expiration of an existing key
func (cm *CacheManager) SetExpire(ctx context.Context, key string, ttl time.Duration) error {
	fullKey := cm.prefix + key

	if err := cm.client.Expire(ctx, fullKey, ttl).Err(); err != nil {
		return fmt.Errorf("failed to set expiration: %w", err)
	}

	return nil
}

// Count returns the count of all cached items
func (cm *CacheManager) Count(ctx context.Context) (int64, error) {
	pattern := cm.prefix + "*"

	keys, err := cm.client.Keys(ctx, pattern).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to count keys: %w", err)
	}

	return int64(len(keys)), nil
}

// Close closes the Redis connection
func (cm *CacheManager) Close() error {
	return cm.client.Close()
}

// Error definitions
var (
	ErrCacheMiss = fmt.Errorf("cache miss")
)

// Public cache key constants
const (
	// Job metadata cache
	KeyJobMetadata = "job:metadata:"
	KeyJobResult   = "job:result:"

	// Platform metadata cache
	KeyPlatformInfo = "platform:info:"

	// Quality presets cache
	KeyQualityPresets = "quality:presets"

	// Extraction metadata cache
	KeyExtractionMetadata = "extraction:metadata:"
)

// CacheTTLs defines default TTL for different cache types
var CacheTTLs = struct {
	JobMetadata        time.Duration
	JobResult          time.Duration
	PlatformInfo       time.Duration
	QualityPresets     time.Duration
	ExtractionMetadata time.Duration
}{
	JobMetadata:        1 * time.Hour,       // Cache job metadata for 1 hour
	JobResult:          24 * time.Hour,      // Cache results for 24 hours
	PlatformInfo:       7 * 24 * time.Hour,  // Cache platform info for 1 week
	QualityPresets:     30 * 24 * time.Hour, // Cache presets for 30 days
	ExtractionMetadata: 12 * time.Hour,      // Cache extraction metadata for 12 hours
}
