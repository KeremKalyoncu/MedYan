package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// URLMetadata stores cached metadata for a URL
type URLMetadata struct {
	URL         string    `json:"url"`
	Title       string    `json:"title"`
	Duration    int       `json:"duration"`
	FileSize    int64     `json:"file_size"`
	Format      string    `json:"format"`
	Platform    string    `json:"platform"`
	CachedAt    time.Time `json:"cached_at"`
	DownloadURL string    `json:"download_url,omitempty"`
}

// DistributedCache provides distributed caching for URL metadata
type DistributedCache struct {
	client *redis.Client
	logger *zap.Logger
	ttl    time.Duration
}

// NewDistributedCache creates a new distributed cache instance
func NewDistributedCache(redisAddr string, logger *zap.Logger) (*DistributedCache, error) {
	// Optimize Redis client with connection pooling
	client := redis.NewClient(&redis.Options{
		Addr:         redisAddr,
		DB:           1,  // Use DB 1 for cache (DB 0 is for job queue)
		PoolSize:     20, // Increased from default 10
		MinIdleConns: 5,
		MaxRetries:   3,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolTimeout:  4 * time.Second,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	logger.Info("Distributed cache initialized with optimized connection pool",
		zap.String("redis_addr", redisAddr),
		zap.Int("pool_size", 20),
	)

	return &DistributedCache{
		client: client,
		logger: logger,
		ttl:    24 * time.Hour, // Cache for 24 hours
	}, nil
}

// hashURL creates a SHA256 hash of the URL for use as cache key
func (dc *DistributedCache) hashURL(url string) string {
	hash := sha256.Sum256([]byte(url))
	return hex.EncodeToString(hash[:])
}

// GetMetadata retrieves cached metadata for a URL
func (dc *DistributedCache) GetMetadata(ctx context.Context, url string) (*URLMetadata, error) {
	key := "url:meta:" + dc.hashURL(url)

	data, err := dc.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, nil // Not found
	}
	if err != nil {
		dc.logger.Warn("Failed to get cached metadata",
			zap.String("url", url),
			zap.Error(err),
		)
		return nil, err
	}

	var metadata URLMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		dc.logger.Error("Failed to unmarshal cached metadata",
			zap.String("url", url),
			zap.Error(err),
		)
		return nil, err
	}

	return &metadata, nil
}

// SetMetadata stores metadata for a URL in cache
func (dc *DistributedCache) SetMetadata(ctx context.Context, metadata *URLMetadata) error {
	key := "url:meta:" + dc.hashURL(metadata.URL)

	metadata.CachedAt = time.Now()

	data, err := json.Marshal(metadata)
	if err != nil {
		dc.logger.Error("Failed to marshal metadata",
			zap.String("url", metadata.URL),
			zap.Error(err),
		)
		return err
	}

	if err := dc.client.Set(ctx, key, data, dc.ttl).Err(); err != nil {
		dc.logger.Error("Failed to cache metadata",
			zap.String("url", metadata.URL),
			zap.Error(err),
		)
		return err
	}

	dc.logger.Debug("Cached URL metadata",
		zap.String("url", metadata.URL),
		zap.String("platform", metadata.Platform),
		zap.Duration("ttl", dc.ttl),
	)

	return nil
}

// InvalidateMetadata removes cached metadata for a URL
func (dc *DistributedCache) InvalidateMetadata(ctx context.Context, url string) error {
	key := "url:meta:" + dc.hashURL(url)
	return dc.client.Del(ctx, key).Err()
}

// IncrementDownloadCount tracks download counts per URL
func (dc *DistributedCache) IncrementDownloadCount(ctx context.Context, url string) (int64, error) {
	key := "url:count:" + dc.hashURL(url)
	count, err := dc.client.Incr(ctx, key).Result()
	if err != nil {
		return 0, err
	}

	// Set expiry on first increment
	if count == 1 {
		dc.client.Expire(ctx, key, 30*24*time.Hour) // 30 days
	}

	return count, nil
}

// GetPopularURLs returns the most frequently downloaded URLs
// Uses SCAN instead of KEYS for production safety (non-blocking)
func (dc *DistributedCache) GetPopularURLs(ctx context.Context, limit int64) (map[string]int64, error) {
	// This is a simplified version - in production, you'd use a sorted set
	pattern := "url:count:*"

	// Use SCAN instead of KEYS to avoid blocking Redis
	var cursor uint64
	var keys []string
	for {
		var batch []string
		var err error
		batch, cursor, err = dc.client.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, err
		}
		keys = append(keys, batch...)
		if cursor == 0 {
			break
		}
	}

	result := make(map[string]int64)

	// Use pipeline for efficient batch operations
	pipe := dc.client.Pipeline()
	cmds := make(map[string]*redis.StringCmd)

	for _, key := range keys {
		cmds[key] = pipe.Get(ctx, key)
	}

	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		return nil, err
	}

	for key, cmd := range cmds {
		count, err := cmd.Int64()
		if err == nil {
			result[key] = count
		}
	}

	return result, nil
}

// Close closes the Redis connection
func (dc *DistributedCache) Close() error {
	return dc.client.Close()
}

// GetMetadataBatch retrieves multiple URL metadata in a single pipeline operation
// This is 10x faster than individual Get calls
func (dc *DistributedCache) GetMetadataBatch(ctx context.Context, urls []string) (map[string]*URLMetadata, error) {
	if len(urls) == 0 {
		return make(map[string]*URLMetadata), nil
	}

	// Use pipeline for batch operations
	pipe := dc.client.Pipeline()
	cmds := make(map[string]*redis.StringCmd, len(urls))

	for _, url := range urls {
		key := "url:meta:" + dc.hashURL(url)
		cmds[url] = pipe.Get(ctx, key)
	}

	// Execute pipeline - single network round-trip
	_, err := pipe.Exec(ctx)
	if err != nil && err != redis.Nil {
		dc.logger.Warn("Pipeline exec failed", zap.Error(err))
	}

	// Parse results
	result := make(map[string]*URLMetadata, len(urls))
	for url, cmd := range cmds {
		data, err := cmd.Bytes()
		if err == redis.Nil {
			continue // Not found, skip
		}
		if err != nil {
			dc.logger.Warn("Failed to get metadata from pipeline",
				zap.String("url", url),
				zap.Error(err),
			)
			continue
		}

		var metadata URLMetadata
		if err := json.Unmarshal(data, &metadata); err != nil {
			dc.logger.Error("Failed to unmarshal cached metadata",
				zap.String("url", url),
				zap.Error(err),
			)
			continue
		}

		result[url] = &metadata
	}

	dc.logger.Debug("Batch metadata fetch completed",
		zap.Int("requested", len(urls)),
		zap.Int("found", len(result)),
	)

	return result, nil
}

// SetMetadataBatch stores multiple URL metadata in a single pipeline operation
func (dc *DistributedCache) SetMetadataBatch(ctx context.Context, metadataList []*URLMetadata) error {
	if len(metadataList) == 0 {
		return nil
	}

	// Use pipeline for batch operations
	pipe := dc.client.Pipeline()

	for _, metadata := range metadataList {
		key := "url:meta:" + dc.hashURL(metadata.URL)
		metadata.CachedAt = time.Now()

		data, err := json.Marshal(metadata)
		if err != nil {
			dc.logger.Error("Failed to marshal metadata",
				zap.String("url", metadata.URL),
				zap.Error(err),
			)
			continue
		}

		pipe.Set(ctx, key, data, dc.ttl)
	}

	// Execute pipeline - single network round-trip
	_, err := pipe.Exec(ctx)
	if err != nil {
		dc.logger.Error("Failed to execute batch cache set", zap.Error(err))
		return err
	}

	dc.logger.Debug("Batch metadata cache completed",
		zap.Int("count", len(metadataList)),
	)

	return nil
}

// InvalidateMetadataBatch removes cached metadata for multiple URLs in a single pipeline
func (dc *DistributedCache) InvalidateMetadataBatch(ctx context.Context, urls []string) error {
	if len(urls) == 0 {
		return nil
	}

	// Use pipeline for batch deletes
	pipe := dc.client.Pipeline()

	for _, url := range urls {
		key := "url:meta:" + dc.hashURL(url)
		pipe.Del(ctx, key)
	}

	// Execute pipeline
	_, err := pipe.Exec(ctx)
	return err
}

// Stats returns cache statistics
func (dc *DistributedCache) Stats(ctx context.Context) (map[string]interface{}, error) {
	info, err := dc.client.Info(ctx, "stats").Result()
	if err != nil {
		return nil, err
	}

	poolStats := dc.client.PoolStats()

	return map[string]interface{}{
		"redis_info":    info,
		"pool_hits":     poolStats.Hits,
		"pool_misses":   poolStats.Misses,
		"pool_timeouts": poolStats.Timeouts,
		"total_conns":   poolStats.TotalConns,
		"idle_conns":    poolStats.IdleConns,
		"stale_conns":   poolStats.StaleConns,
	}, nil
}
