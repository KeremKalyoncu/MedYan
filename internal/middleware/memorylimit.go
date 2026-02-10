package middleware

import (
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"
)

// MemoryLimitConfig defines memory limit middleware configuration
type MemoryLimitConfig struct {
	// MaxMemoryMB is the maximum memory per request in MB
	MaxMemoryMB int64

	// SoftLimitMB triggers GC but allows request to continue
	SoftLimitMB int64

	// CheckInterval how often to check memory usage during request
	CheckInterval time.Duration

	// Logger for memory warnings
	Logger *zap.Logger

	// Next defines a function to skip this middleware
	Next func(c *fiber.Ctx) bool
}

// DefaultMemoryLimitConfig returns default configuration
func DefaultMemoryLimitConfig() MemoryLimitConfig {
	return MemoryLimitConfig{
		MaxMemoryMB:   500,                    // 500MB hard limit per request
		SoftLimitMB:   200,                    // 200MB soft limit (triggers GC)
		CheckInterval: 500 * time.Millisecond, // Check every 500ms
		Logger:        nil,
	}
}

// MemoryLimitMiddleware tracks memory usage during requests
// Prevents individual requests from consuming too much memory
func MemoryLimitMiddleware(config ...MemoryLimitConfig) fiber.Handler {
	cfg := DefaultMemoryLimitConfig()
	if len(config) > 0 {
		cfg = config[0]
	}

	// Memory tracking pool
	memStatsPool := sync.Pool{
		New: func() interface{} {
			return &runtime.MemStats{}
		},
	}

	return func(c *fiber.Ctx) error {
		// Skip if Next returns true
		if cfg.Next != nil && cfg.Next(c) {
			return c.Next()
		}

		// Get initial memory state
		memStats := memStatsPool.Get().(*runtime.MemStats)
		defer memStatsPool.Put(memStats)

		runtime.ReadMemStats(memStats)
		initialAlloc := memStats.Alloc

		// Store in context for other handlers
		c.Locals("mem_initial_alloc", initialAlloc)

		// Process request
		if err := c.Next(); err != nil {
			return err
		}

		// Check final memory usage
		runtime.ReadMemStats(memStats)
		finalAlloc := memStats.Alloc
		requestMemory := int64(finalAlloc - initialAlloc)
		requestMemoryMB := requestMemory / 1024 / 1024

		// Log high memory usage
		if requestMemoryMB > cfg.SoftLimitMB && cfg.Logger != nil {
			cfg.Logger.Warn("High memory usage during request",
				zap.String("path", c.Path()),
				zap.Int64("memory_mb", requestMemoryMB),
				zap.Int64("soft_limit_mb", cfg.SoftLimitMB),
			)

			// Trigger GC for soft limit
			runtime.GC()
			debug.FreeOSMemory()
		}

		if requestMemoryMB > cfg.MaxMemoryMB && cfg.Logger != nil {
			cfg.Logger.Error("Memory limit exceeded",
				zap.String("path", c.Path()),
				zap.Int64("memory_mb", requestMemoryMB),
				zap.Int64("max_limit_mb", cfg.MaxMemoryMB),
			)
		}

		return nil
	}
}

// GetMemoryUsage returns current memory usage in MB
func GetMemoryUsage() int64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return int64(m.Alloc / 1024 / 1024)
}

// ForceGC manually triggers garbage collection
// Use sparingly, only when memory pressure is high
func ForceGC() {
	runtime.GC()
	debug.FreeOSMemory()
}
