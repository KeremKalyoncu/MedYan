package handlers

import (
	"context"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"github.com/KeremKalyoncu/MedYan/internal/cache"
	"github.com/KeremKalyoncu/MedYan/internal/queue"
)

// HealthHandler provides health check endpoints
type HealthHandler struct {
	queue  *queue.Client
	cache  *cache.CacheManager
	logger *zap.Logger
}

// NewHealthHandler creates a health handler
func NewHealthHandler(queueClient *queue.Client, cacheManager *cache.CacheManager, logger *zap.Logger) *HealthHandler {
	return &HealthHandler{
		queue:  queueClient,
		cache:  cacheManager,
		logger: logger,
	}
}

// BasicHealth returns simple healthy status (for load balancers)
func (h *HealthHandler) BasicHealth(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status": "healthy",
		"time":   time.Now().Unix(),
	})
}

// DetailedHealth returns comprehensive health status
func (h *HealthHandler) DetailedHealth(c *fiber.Ctx) error {
	ctx := context.Background()

	health := fiber.Map{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"version":   "1.0.0",
		"checks":    fiber.Map{},
	}

	checks := health["checks"].(fiber.Map)
	allHealthy := true

	// Check Redis Queue
	queueStatus := "healthy"
	if _, err := h.queue.GetJobStatus(ctx, "health-check"); err != nil && err.Error() != "job not found: health-check" {
		queueStatus = "unhealthy"
		allHealthy = false
		h.logger.Warn("Redis queue health check failed", zap.Error(err))
	}
	checks["redis_queue"] = fiber.Map{
		"status": queueStatus,
	}

	// Check Cache
	cacheStatus := "healthy"
	testKey := "health:check:" + time.Now().Format("20060102")
	if err := h.cache.Set(ctx, testKey, "ok", 10*time.Second); err != nil {
		cacheStatus = "unhealthy"
		allHealthy = false
		h.logger.Warn("Cache health check failed", zap.Error(err))
	}
	checks["cache"] = fiber.Map{
		"status": cacheStatus,
	}

	// Check yt-dlp availability
	ytdlpStatus := "healthy"
	// You can add actual yt-dlp version check here
	checks["ytdlp"] = fiber.Map{
		"status": ytdlpStatus,
	}

	// Check FFmpeg availability
	ffmpegStatus := "healthy"
	// You can add actual FFmpeg version check here
	checks["ffmpeg"] = fiber.Map{
		"status": ffmpegStatus,
	}

	// Update overall status
	if !allHealthy {
		health["status"] = "degraded"
	}

	// Set appropriate HTTP status code
	statusCode := fiber.StatusOK
	if health["status"] == "degraded" {
		statusCode = fiber.StatusServiceUnavailable
	}

	return c.Status(statusCode).JSON(health)
}

// Readiness returns whether service is ready to accept traffic
func (h *HealthHandler) Readiness(c *fiber.Ctx) error {
	// Check if critical services are available
	ctx := context.Background()

	// Test Redis
	if _, err := h.queue.GetJobStatus(ctx, "readiness-check"); err != nil && err.Error() != "job not found: readiness-check" {
		return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
			"ready":   false,
			"message": "Redis not available",
		})
	}

	return c.JSON(fiber.Map{
		"ready": true,
	})
}

// Liveness returns whether service is alive (for k8s liveness probe)
func (h *HealthHandler) Liveness(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"alive": true,
	})
}
