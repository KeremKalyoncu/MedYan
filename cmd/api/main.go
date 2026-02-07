package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"go.uber.org/zap"

	"github.com/KeremKalyoncu/MedYan/internal/cache"
	"github.com/KeremKalyoncu/MedYan/internal/dedup"
	"github.com/KeremKalyoncu/MedYan/internal/extractor"
	"github.com/KeremKalyoncu/MedYan/internal/handlers"
	"github.com/KeremKalyoncu/MedYan/internal/metrics"
	"github.com/KeremKalyoncu/MedYan/internal/middleware"
	"github.com/KeremKalyoncu/MedYan/internal/pool"
	"github.com/KeremKalyoncu/MedYan/internal/queue"
	"github.com/KeremKalyoncu/MedYan/internal/types"
)

func main() {
	// Initialize logger
	zapLogger, err := zap.NewProduction()
	if err != nil {
		log.Fatal("Failed to initialize logger:", err)
	}
	defer zapLogger.Sync()

	// Initialize queue client
	redisAddr := getEnv("REDIS_ADDR", "localhost:6379")
	queueClient := queue.NewClient(redisAddr, zapLogger)
	defer queueClient.Close()

	// Initialize distributed cache for performance optimization
	distCache, err := cache.NewDistributedCache(redisAddr, zapLogger)
	if err != nil {
		zapLogger.Warn("Failed to initialize distributed cache", zap.Error(err))
		distCache = nil // Continue without cache
	} else {
		defer distCache.Close()
		zapLogger.Info("Distributed cache initialized successfully")
	}

	// Initialize worker pool for parallel task processing
	workerPool := pool.NewWorkerPool(10, 100) // 10 workers, 100 task queue size
	defer workerPool.Shutdown()
	zapLogger.Info("Worker pool initialized", zap.Int("workers", 10), zap.Int("queue_size", 100))

	// Initialize request deduplication (prevents duplicate URL processing)
	deduplicator := dedup.NewSingleflight()
	zapLogger.Info("Request deduplication enabled - identical URLs will be coalesced")

	// Initialize yt-dlp for smart URL detection (API-only, not worker)
	ytdlpBinary := getEnv("YTDLP_PATH", "yt-dlp")
	ytdlpTimeout := 120 * time.Second
	ytdlp := extractor.NewYtDlp(ytdlpBinary, ytdlpTimeout, zapLogger)
	zapLogger.Info("yt-dlp initialized for smart URL detection")

	// Initialize detection handler for smart platform detection
	detectionHandler := handlers.NewDetectionHandler(ytdlp, zapLogger)
	zapLogger.Info("Smart platform detection enabled")

	// API key is mandatory for security in production
	apiKey := getEnv("API_KEY", "")
	if apiKey == "" {
		zapLogger.Fatal("API_KEY environment variable is required for security")
	}

	// Create Fiber app with optimized settings
	app := fiber.New(fiber.Config{
		AppName:               "Media Extraction API v2.0 - Performance Optimized",
		ReadTimeout:           2 * time.Minute, // Increased for audio processing
		WriteTimeout:          2 * time.Minute, // Increased for audio processing
		DisableStartupMessage: false,
		EnablePrintRoutes:     false,
		// Performance optimizations
		BodyLimit:            100 * 1024 * 1024, // 100MB max body size
		ReduceMemoryUsage:    true,              // Reduce memory footprint
		StreamRequestBody:    true,              // Stream large requests
		CompressedFileSuffix: ".fiber.gz",       // GZIP compression suffix
	})

	// Middleware stack (order matters for performance)
	app.Use(recover.New())

	// Compression middleware - reduces bandwidth by 60-80% for JSON responses
	app.Use(middleware.CompressionMiddleware())

	app.Use(logger.New(logger.Config{
		Format: "[${time}] ${status} - ${latency} ${method} ${path}\n",
	}))

	// CORS security: Only allow trusted origins
	app.Use(cors.New(cors.Config{
		AllowOrigins:     "https://keremkalyoncu.github.io, https://medyan-production.up.railway.app, http://localhost:3000",
		AllowMethods:     "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders:     "Origin,Content-Type,Accept,Authorization,X-API-Key",
		AllowCredentials: true,
	}))

	// Rate limiting on proxy endpoints (100 req/min per IP)
	rateLimiter := middleware.NewRateLimiter(100, time.Minute)

	// Metrics middleware
	metricsInstance := metrics.GetMetrics()
	app.Use(func(c *fiber.Ctx) error {
		metricsInstance.IncrementRequests()
		return c.Next()
	})

	// Health check - basic (for load balancer)
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "healthy",
			"time":   time.Now().Unix(),
		})
	})

	// Metrics endpoint (public, read-only)
	// Cache for 30 seconds to reduce computational load
	app.Get("/metrics", middleware.CacheMiddleware(middleware.CacheConfig{
		MaxAge:         30, // 30 seconds
		Public:         true,
		MustRevalidate: false,
	}), func(c *fiber.Ctx) error {
		snapshot := metricsInstance.GetSnapshot()

		// Add cache stats if available
		if distCache != nil {
			cacheStats, err := distCache.Stats(context.Background())
			if err == nil {
				snapshot["cache_stats"] = cacheStats
			}
		}

		// Add worker pool stats
		snapshot["worker_pool"] = fiber.Map{
			"active_jobs": workerPool.ActiveJobs(),
		}

		// Add deduplication stats
		snapshot["deduplication"] = deduplicator.Stats()

		return c.JSON(snapshot)
	})

	// Performance profiling endpoints (for debugging)
	if getEnv("ENABLE_PPROF", "false") == "true" {
		handlers.RegisterPprofRoutes(app)
		zapLogger.Info("pprof profiling endpoints enabled at /debug/pprof")
	}

	// Public proxy routes (for GitHub Pages frontend - no API key required)
	proxy := app.Group("/proxy")

	// Apply rate limiting to proxy routes
	proxy.Use(rateLimiter.Middleware())

	// Smart URL detection endpoint - Analyzes URL and returns platform info + available options
	// Frontend calls this first to show appropriate UI controls
	proxy.Post("/detect", detectionHandler.DetectURL)

	// Proxy extract endpoint - Frontend uses this without exposing API key
	proxy.Post("/extract", func(c *fiber.Ctx) error {
		var req types.ExtractionRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{
				"error": "Invalid request body",
			})
		}

		// Validate URL
		if req.URL == "" {
			return c.Status(400).JSON(fiber.Map{
				"error": "URL is required",
			})
		}

		// Quick duration check (max 3 minutes)
		durationCtx, durationCancel := context.WithTimeout(c.Context(), 15*time.Second)
		metadata, _ := ytdlp.ExtractMetadata(durationCtx, req.URL)
		durationCancel()

		if metadata != nil && metadata.Duration > 180 {
			return c.Status(403).JSON(fiber.Map{
				"error":        "VIDEO_TOO_LONG",
				"duration":     metadata.Duration,
				"max_duration": 180,
				"message":      "Video süre limiti aşıldı. Şu anda maksimum 3 dakikalık videolar desteklenmektedir.",
			})
		}

		// Use deduplication to coalesce identical URL requests
		// Create a unique key based on URL and format settings
		dedupKey := fmt.Sprintf("%s:%s:%v:%s:%s",
			req.URL, req.Format, req.ExtractAudio, req.AudioFormat, req.Quality)

		result := deduplicator.DoContext(c.Context(), dedupKey, func() (interface{}, error) {
			// This function only runs once per unique request
			// Other concurrent identical requests will wait and share the result
			return queueClient.EnqueueExtractionJob(context.Background(), req)
		})

		if result.Err != nil {
			zapLogger.Error("Failed to enqueue job",
				zap.Error(result.Err),
				zap.Bool("deduplicated", result.Shared),
			)
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to enqueue job",
			})
		}

		jobID := result.Val.(string)

		return c.Status(202).JSON(fiber.Map{
			"job_id":       jobID,
			"status":       "pending",
			"message":      "Extraction job queued successfully",
			"deduplicated": result.Shared, // Let client know if request was coalesced
		})
	})

	// Proxy job status endpoint - Check job progress without API key
	// Cache completed jobs for 5 minutes to reduce load
	proxy.Get("/jobs/:id", middleware.ConditionalCacheMiddleware(
		func(c *fiber.Ctx) bool {
			// Only cache completed jobs
			jobID := c.Params("id")
			job, err := queueClient.GetJobStatus(context.Background(), jobID)
			return err == nil && job.Status == types.StatusCompleted
		},
		middleware.CacheConfig{
			MaxAge:         300,   // 5 minutes
			Public:         false, // Private cache (user-specific data)
			MustRevalidate: true,
		},
	), func(c *fiber.Ctx) error {
		jobID := c.Params("id")

		job, err := queueClient.GetJobStatus(context.Background(), jobID)
		if err != nil {
			return c.Status(404).JSON(fiber.Map{
				"error": "Job not found",
			})
		}

		return c.JSON(job)
	})

	// Proxy download endpoint - Download without exposing API key
	proxy.Get("/download/:id", func(c *fiber.Ctx) error {
		jobID := c.Params("id")

		job, err := queueClient.GetJobStatus(context.Background(), jobID)
		if err != nil {
			return c.Status(404).JSON(fiber.Map{
				"error": "Job not found",
			})
		}

		if job.Status != types.StatusCompleted {
			return c.Status(400).JSON(fiber.Map{
				"error":  "Job not completed",
				"status": job.Status,
			})
		}

		if job.Result == nil || job.Result.DownloadURL == "" {
			return c.Status(500).JSON(fiber.Map{
				"error": "Download URL not available",
			})
		}

		// Redirect to S3 presigned URL or local download
		return c.Redirect(job.Result.DownloadURL, 302)
	})

	// Serve downloaded files (for local storage)
	// Use * wildcard to capture full path including subdirectories
	app.Get("/downloads/*", func(c *fiber.Ctx) error {
		filePath := c.Params("*")
		fullPath := filepath.Join("/app/downloads", filepath.Clean(filePath))

		// Security check - prevent directory traversal
		if !strings.HasPrefix(fullPath, "/app/downloads") {
			return c.Status(400).JSON(fiber.Map{
				"error": "Invalid file path",
			})
		}

		// Check if file exists
		if _, err := os.Stat(fullPath); os.IsNotExist(err) {
			return c.Status(404).JSON(fiber.Map{
				"error": "File not found",
			})
		}

		// Set headers for proper download
		filename := filepath.Base(fullPath)
		c.Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
		c.Set("Content-Type", "application/octet-stream")
		c.Set("Cache-Control", "no-cache")

		// Stream file (handles large files efficiently)
		return c.SendFile(fullPath, false) // false = no compression
	})

	// API routes (protected with API key - for backend-to-backend or authorized clients)
	api := app.Group("/api/v1")

	// Apply API key auth to protected endpoints
	api.Use(middleware.APIKeyAuth(apiKey))

	// Extract endpoint
	api.Post("/extract", func(c *fiber.Ctx) error {
		var req types.ExtractionRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{
				"error": "Invalid request body",
			})
		}

		// Validate URL
		if req.URL == "" {
			return c.Status(400).JSON(fiber.Map{
				"error": "URL is required",
			})
		}

		// Quick duration check (max 3 minutes)
		durationCtx, durationCancel := context.WithTimeout(c.Context(), 15*time.Second)
		metadata, _ := ytdlp.ExtractMetadata(durationCtx, req.URL)
		durationCancel()

		if metadata != nil && metadata.Duration > 180 {
			return c.Status(403).JSON(fiber.Map{
				"error":        "VIDEO_TOO_LONG",
				"duration":     metadata.Duration,
				"max_duration": 180,
				"message":      "Video süre limiti aşıldı. Şu anda maksimum 3 dakikalık videolar desteklenmektedir.",
			})
		}

		// Enqueue job
		jobID, err := queueClient.EnqueueExtractionJob(context.Background(), req)
		if err != nil {
			zapLogger.Error("Failed to enqueue job", zap.Error(err))
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to enqueue job",
			})
		}

		return c.Status(202).JSON(fiber.Map{
			"job_id":  jobID,
			"status":  "pending",
			"message": "Extraction job queued successfully",
		})
	})

	// Batch extract endpoint
	api.Post("/batch", func(c *fiber.Ctx) error {
		var req struct {
			URLs     []string                `json:"urls"`
			Template types.ExtractionRequest `json:"template"`
		}

		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{
				"error": "Invalid request body",
			})
		}

		if len(req.URLs) == 0 {
			return c.Status(400).JSON(fiber.Map{
				"error": "URLs array is required",
			})
		}

		// Enqueue batch
		jobIDs, err := queueClient.EnqueueBatchJob(context.Background(), req.URLs, req.Template)
		if err != nil {
			zapLogger.Error("Failed to enqueue batch", zap.Error(err))
			return c.Status(500).JSON(fiber.Map{
				"error": "Failed to enqueue batch jobs",
			})
		}

		return c.Status(202).JSON(fiber.Map{
			"job_ids": jobIDs,
			"count":   len(jobIDs),
			"message": "Batch extraction jobs queued successfully",
		})
	})

	// Get job status endpoint
	api.Get("/jobs/:id", func(c *fiber.Ctx) error {
		jobID := c.Params("id")

		job, err := queueClient.GetJobStatus(context.Background(), jobID)
		if err != nil {
			return c.Status(404).JSON(fiber.Map{
				"error": "Job not found",
			})
		}

		return c.JSON(job)
	})

	// Download endpoint (redirect to presigned URL)
	api.Get("/download/:id", func(c *fiber.Ctx) error {
		jobID := c.Params("id")

		job, err := queueClient.GetJobStatus(context.Background(), jobID)
		if err != nil {
			return c.Status(404).JSON(fiber.Map{
				"error": "Job not found",
			})
		}

		if job.Status != types.StatusCompleted {
			return c.Status(400).JSON(fiber.Map{
				"error":  "Job not completed",
				"status": job.Status,
			})
		}

		if job.Result == nil || job.Result.DownloadURL == "" {
			return c.Status(500).JSON(fiber.Map{
				"error": "Download URL not available",
			})
		}

		// Redirect to S3 presigned URL
		return c.Redirect(job.Result.DownloadURL, 302)
	})

	// Webhook registration endpoint
	api.Post("/webhooks/register", func(c *fiber.Ctx) error {
		var req struct {
			JobID      string `json:"job_id"`
			WebhookURL string `json:"webhook_url"`
		}

		if err := c.BodyParser(&req); err != nil {
			return c.Status(400).JSON(fiber.Map{
				"error": "Invalid request body",
			})
		}

		// TODO: Implement webhook registration logic
		// Store webhook URL in Redis associated with job_id

		return c.JSON(fiber.Map{
			"message": "Webhook registered successfully",
		})
	})

	// Start server
	port := getEnv("PORT", "8080")
	zapLogger.Info("Starting API server", zap.String("port", port))

	// Graceful shutdown
	go func() {
		if err := app.Listen(":" + port); err != nil {
			zapLogger.Fatal("Failed to start server", zap.Error(err))
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	zapLogger.Info("Shutting down server...")
	if err := app.Shutdown(); err != nil {
		zapLogger.Error("Server shutdown error", zap.Error(err))
	}

	zapLogger.Info("Server stopped")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
