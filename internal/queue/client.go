package queue

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"

	"github.com/KeremKalyoncu/MedYan/internal/types"
)

// Task types
const (
	TypeExtraction = "extraction:media"
	TypeBatch      = "extraction:batch"
)

// Client wraps Asynq client for job enqueueing
type Client struct {
	asynq  *asynq.Client
	redis  *redis.Client
	logger *zap.Logger
}

// NewClient creates a new queue client
func NewClient(redisAddr string, logger *zap.Logger) *Client {
	asynqClient := asynq.NewClient(asynq.RedisClientOpt{Addr: redisAddr})

	// Optimize Redis client with connection pooling
	redisClient := redis.NewClient(&redis.Options{
		Addr:         redisAddr,
		PoolSize:     20, // Increased connection pool (default: 10)
		MinIdleConns: 5,  // Keep minimum idle connections
		MaxRetries:   3,  // Retry failed commands
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		// Enable connection pooling optimizations
		PoolTimeout: 4 * time.Second,
	})

	return &Client{
		asynq:  asynqClient,
		redis:  redisClient,
		logger: logger,
	}
}

// EnqueueExtractionJob enqueues a media extraction job with deduplication
func (c *Client) EnqueueExtractionJob(ctx context.Context, req types.ExtractionRequest) (string, error) {
	jobID := uuid.New().String()

	job := types.ExtractionJob{
		ID:        jobID,
		Request:   req,
		Status:    types.StatusPending,
		Progress:  0,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Store job metadata in Redis
	if err := c.storeJobMetadata(ctx, &job); err != nil {
		return "", fmt.Errorf("failed to store job metadata: %w", err)
	}

	// Prepare task payload
	payload, err := json.Marshal(job)
	if err != nil {
		return "", fmt.Errorf("failed to marshal job: %w", err)
	}

	// Determine queue based on quality
	queue := c.getQueueForQuality(req.Quality)

	// Create Asynq task with deduplication
	task := asynq.NewTask(TypeExtraction, payload)
	taskOpts := []asynq.Option{
		asynq.Queue(queue),
		asynq.MaxRetry(3),
		asynq.Timeout(10 * time.Minute),
		asynq.Retention(7 * 24 * time.Hour),
		// Deduplication: same URL within 5 min (optimized from 24h)
		// Shorter TTL reduces memory usage and allows retry faster
		asynq.Unique(5 * time.Minute),
		asynq.TaskID(jobID),
	}

	info, err := c.asynq.EnqueueContext(ctx, task, taskOpts...)
	if err != nil {
		return "", fmt.Errorf("failed to enqueue task: %w", err)
	}

	c.logger.Info("Job enqueued",
		zap.String("job_id", jobID),
		zap.String("url", req.URL),
		zap.String("queue", info.Queue),
	)

	return jobID, nil
}

// EnqueueBatchJob enqueues multiple extraction jobs
func (c *Client) EnqueueBatchJob(ctx context.Context, urls []string, template types.ExtractionRequest) ([]string, error) {
	jobIDs := make([]string, 0, len(urls))

	for _, url := range urls {
		req := template
		req.URL = url

		jobID, err := c.EnqueueExtractionJob(ctx, req)
		if err != nil {
			c.logger.Error("Failed to enqueue batch job",
				zap.String("url", url),
				zap.Error(err),
			)
			continue
		}

		jobIDs = append(jobIDs, jobID)
	}

	return jobIDs, nil
}

// GetJobStatus retrieves the current status of a job
func (c *Client) GetJobStatus(ctx context.Context, jobID string) (*types.ExtractionJob, error) {
	key := fmt.Sprintf("job:%s", jobID)
	data, err := c.redis.Get(ctx, key).Bytes()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("job not found: %s", jobID)
		}
		return nil, fmt.Errorf("failed to get job: %w", err)
	}

	var job types.ExtractionJob
	if err := json.Unmarshal(data, &job); err != nil {
		return nil, fmt.Errorf("failed to unmarshal job: %w", err)
	}

	return &job, nil
}

// UpdateJobStatus updates the status of a job
func (c *Client) UpdateJobStatus(ctx context.Context, jobID string, status types.JobStatus, progress int, errorMsg string) error {
	job, err := c.GetJobStatus(ctx, jobID)
	if err != nil {
		return err
	}

	job.Status = status
	job.Progress = progress
	job.UpdatedAt = time.Now()
	if errorMsg != "" {
		job.Error = errorMsg
	}

	return c.storeJobMetadata(ctx, job)
}

// UpdateJobResult updates the job with extraction result
func (c *Client) UpdateJobResult(ctx context.Context, jobID string, result *types.ExtractionResult, metadata *types.MediaMetadata) error {
	job, err := c.GetJobStatus(ctx, jobID)
	if err != nil {
		return err
	}

	job.Status = types.StatusCompleted
	job.Progress = 100
	job.Result = result
	job.Metadata = metadata
	job.UpdatedAt = time.Now()

	if err := c.storeJobMetadata(ctx, job); err != nil {
		return err
	}

	// Trigger webhook if configured
	if job.Request.WebhookURL != "" {
		go c.triggerWebhook(job)
	}

	return nil
}

// storeJobMetadata stores job metadata in Redis
func (c *Client) storeJobMetadata(ctx context.Context, job *types.ExtractionJob) error {
	key := fmt.Sprintf("job:%s", job.ID)
	data, err := json.Marshal(job)
	if err != nil {
		return err
	}

	// Store with 7-day expiry
	return c.redis.Set(ctx, key, data, 7*24*time.Hour).Err()
}

// getQueueForQuality determines the priority queue based on quality setting
func (c *Client) getQueueForQuality(quality string) string {
	switch quality {
	case "4k":
		return "critical"
	case "best", "1080p":
		return "default"
	default:
		return "low"
	}
}

// triggerWebhook sends a POST request to the webhook URL (non-blocking)
func (c *Client) triggerWebhook(job *types.ExtractionJob) {
	// Skip if no webhook URL provided
	if job.Request.WebhookURL == "" {
		return
	}

	// Run in goroutine to avoid blocking
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		// Prepare webhook payload
		payload := map[string]interface{}{
			"job_id":     job.ID,
			"status":     job.Status,
			"url":        job.Request.URL,
			"format":     job.Request.Format,
			"error":      job.Error,
			"created_at": job.CreatedAt,
			"updated_at": job.UpdatedAt,
		}

		// Add result data if successful
		if job.Status == "completed" && job.Result != nil {
			payload["download_url"] = job.Result.DownloadURL
			payload["size_bytes"] = job.Result.SizeBytes
			payload["filename"] = job.Result.Filename
			payload["format"] = job.Result.Format
			payload["expires_at"] = job.Result.ExpiresAt
		}

		// Add metadata if available
		if job.Metadata != nil {
			payload["title"] = job.Metadata.Title
			payload["duration"] = job.Metadata.Duration
			payload["platform"] = job.Metadata.Platform
		}

		jsonData, err := json.Marshal(payload)
		if err != nil {
			c.logger.Error("Failed to marshal webhook payload",
				zap.String("job_id", job.ID),
				zap.Error(err),
			)
			return
		}

		// Create HTTP request
		req, err := http.NewRequestWithContext(ctx, "POST", job.Request.WebhookURL, bytes.NewBuffer(jsonData))
		if err != nil {
			c.logger.Error("Failed to create webhook request",
				zap.String("job_id", job.ID),
				zap.Error(err),
			)
			return
		}

		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "MediaExtraction-Webhook/1.0")

		// Send request with retry logic (3 attempts)
		client := &http.Client{Timeout: 10 * time.Second}
		var lastErr error
		for attempt := 1; attempt <= 3; attempt++ {
			resp, err := client.Do(req)
			if err != nil {
				lastErr = err
				c.logger.Warn("Webhook request failed, retrying...",
					zap.String("job_id", job.ID),
					zap.Int("attempt", attempt),
					zap.Error(err),
				)
				time.Sleep(time.Duration(attempt) * time.Second) // Exponential backoff
				continue
			}
			defer resp.Body.Close()

			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				c.logger.Info("Webhook triggered successfully",
					zap.String("job_id", job.ID),
					zap.String("webhook_url", job.Request.WebhookURL),
					zap.Int("status_code", resp.StatusCode),
				)
				return
			}

			lastErr = fmt.Errorf("webhook returned status %d", resp.StatusCode)
			c.logger.Warn("Webhook returned non-2xx status, retrying...",
				zap.String("job_id", job.ID),
				zap.Int("status_code", resp.StatusCode),
				zap.Int("attempt", attempt),
			)
			time.Sleep(time.Duration(attempt) * time.Second)
		}

		c.logger.Error("Webhook failed after 3 attempts",
			zap.String("job_id", job.ID),
			zap.String("webhook_url", job.Request.WebhookURL),
			zap.Error(lastErr),
		)
	}()
}

// Close closes the client connections
func (c *Client) Close() error {
	if err := c.asynq.Close(); err != nil {
		return err
	}
	return c.redis.Close()
}

// GetRedis returns the underlying redis client
func (c *Client) GetRedis() *redis.Client {
	return c.redis
}
