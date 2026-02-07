package middleware

import (
	"crypto/md5"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
)

// CacheConfig holds cache middleware configuration
type CacheConfig struct {
	// MaxAge is the cache duration in seconds (for Cache-Control header)
	MaxAge int
	// Public allows caching by CDNs and proxies if true
	Public bool
	// MustRevalidate forces revalidation after cache expires
	MustRevalidate bool
	// NoTransform prevents proxies from modifying response
	NoTransform bool
}

// DefaultCacheConfig returns sensible defaults
func DefaultCacheConfig() CacheConfig {
	return CacheConfig{
		MaxAge:         300, // 5 minutes
		Public:         true,
		MustRevalidate: true,
		NoTransform:    true,
	}
}

// CacheMiddleware adds ETag and cache headers for conditional requests
// Supports 304 Not Modified responses to save bandwidth
func CacheMiddleware(config ...CacheConfig) fiber.Handler {
	cfg := DefaultCacheConfig()
	if len(config) > 0 {
		cfg = config[0]
	}

	return func(c *fiber.Ctx) error {
		// Skip for non-GET requests
		if c.Method() != fiber.MethodGet {
			return c.Next()
		}

		// Store original send function
		originalSend := c.Response().BodyWriter()

		// Capture response body
		var responseBody []byte
		var statusCode int

		// Continue processing
		err := c.Next()
		if err != nil {
			return err
		}

		// Get response data
		responseBody = c.Response().Body()
		statusCode = c.Response().StatusCode()

		// Only cache successful responses
		if statusCode < 200 || statusCode >= 300 {
			return nil
		}

		// Generate ETag from response body
		etag := generateETag(responseBody)

		// Set ETag header
		c.Set("ETag", etag)

		// Set Last-Modified header (current time)
		lastModified := time.Now().UTC().Format(time.RFC1123)
		c.Set("Last-Modified", lastModified)

		// Build Cache-Control header
		cacheControl := buildCacheControl(cfg)
		c.Set("Cache-Control", cacheControl)

		// Check if client has cached version
		clientETag := c.Get("If-None-Match")
		clientModifiedSince := c.Get("If-Modified-Since")

		// ETag match - return 304 Not Modified
		if clientETag != "" && clientETag == etag {
			c.Status(fiber.StatusNotModified)
			c.Response().SetBodyRaw(nil)
			return nil
		}

		// If-Modified-Since check
		if clientModifiedSince != "" {
			clientTime, err := time.Parse(time.RFC1123, clientModifiedSince)
			if err == nil {
				serverTime, _ := time.Parse(time.RFC1123, lastModified)
				if !serverTime.After(clientTime) {
					c.Status(fiber.StatusNotModified)
					c.Response().SetBodyRaw(nil)
					return nil
				}
			}
		}

		// Response is fresh, send it normally
		_, _ = originalSend.Write(responseBody)
		return nil
	}
}

// generateETag creates ETag from response body using MD5 hash
func generateETag(body []byte) string {
	hash := md5.Sum(body)
	return `"` + hex.EncodeToString(hash[:]) + `"`
}

// buildCacheControl constructs Cache-Control header value
func buildCacheControl(config CacheConfig) string {
	directives := []string{}

	if config.Public {
		directives = append(directives, "public")
	} else {
		directives = append(directives, "private")
	}

	if config.MaxAge > 0 {
		directives = append(directives, "max-age="+strconv.Itoa(config.MaxAge))
	}

	if config.MustRevalidate {
		directives = append(directives, "must-revalidate")
	}

	if config.NoTransform {
		directives = append(directives, "no-transform")
	}

	result := ""
	for i, dir := range directives {
		if i > 0 {
			result += ", "
		}
		result += dir
	}

	return result
}

// NoCacheMiddleware disables caching for specific routes
func NoCacheMiddleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		c.Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
		c.Set("Pragma", "no-cache")
		c.Set("Expires", "0")
		return c.Next()
	}
}

// ConditionalCacheMiddleware applies caching based on condition
func ConditionalCacheMiddleware(shouldCache func(*fiber.Ctx) bool, config CacheConfig) fiber.Handler {
	cacheHandler := CacheMiddleware(config)
	noCacheHandler := NoCacheMiddleware()

	return func(c *fiber.Ctx) error {
		if shouldCache(c) {
			return cacheHandler(c)
		}
		return noCacheHandler(c)
	}
}
