package middleware

import (
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
)

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	clients map[string]*clientBucket
	mu      sync.RWMutex
	rate    int           // requests per window
	window  time.Duration // time window
}

type clientBucket struct {
	tokens     int
	lastRefill time.Time
}

// NewRateLimiter creates a rate limiter (e.g., 100 requests per minute)
func NewRateLimiter(requestsPerWindow int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		clients: make(map[string]*clientBucket),
		rate:    requestsPerWindow,
		window:  window,
	}

	// Cleanup goroutine - remove stale clients every 5 minutes
	go rl.cleanup()

	return rl
}

// Middleware returns Fiber middleware function
func (rl *RateLimiter) Middleware() fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get client IP
		clientIP := c.IP()

		if !rl.allow(clientIP) {
			return c.Status(429).JSON(fiber.Map{
				"error":       "Rate limit exceeded",
				"message":     "Too many requests. Please try again later.",
				"retry_after": int(rl.window.Seconds()),
			})
		}

		return c.Next()
	}
}

// allow checks if client can make a request
func (rl *RateLimiter) allow(clientID string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	bucket, exists := rl.clients[clientID]
	if !exists {
		bucket = &clientBucket{
			tokens:     rl.rate,
			lastRefill: now,
		}
		rl.clients[clientID] = bucket
	}

	// Refill tokens based on time elapsed
	elapsed := now.Sub(bucket.lastRefill)
	if elapsed >= rl.window {
		bucket.tokens = rl.rate
		bucket.lastRefill = now
	}

	// Check if client has tokens
	if bucket.tokens > 0 {
		bucket.tokens--
		return true
	}

	return false
}

// cleanup removes stale client entries
func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		now := time.Now()
		for clientID, bucket := range rl.clients {
			if now.Sub(bucket.lastRefill) > 10*time.Minute {
				delete(rl.clients, clientID)
			}
		}
		rl.mu.Unlock()
	}
}
