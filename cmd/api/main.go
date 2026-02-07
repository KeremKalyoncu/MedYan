package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"go.uber.org/zap"

	"github.com/gsker/media-extraction-saas/internal/middleware"
	"github.com/gsker/media-extraction-saas/internal/queue"
	"github.com/gsker/media-extraction-saas/internal/types"
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

	// Get API key from env (or use default for development)
	apiKey := getEnv("API_KEY", "dev-key-change-me-in-production")

	// Create Fiber app
	app := fiber.New(fiber.Config{
		AppName:      "Media Extraction API",
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
	})

	// Middleware
	app.Use(recover.New())
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowMethods: "GET,POST,PUT,DELETE,OPTIONS",
		AllowHeaders: "Origin,Content-Type,Accept,Authorization",
	}))

	// Health check
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "healthy",
			"time":   time.Now().Unix(),
		})
	})

	// API routes
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
