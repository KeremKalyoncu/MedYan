package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/KeremKalyoncu/MedYan/internal/extractor"
	"github.com/KeremKalyoncu/MedYan/internal/handlers"
	"github.com/KeremKalyoncu/MedYan/internal/queue"
	"github.com/KeremKalyoncu/MedYan/pkg/storage"
)

func main() {
	// Initialize logger
	zapLogger, err := zap.NewProduction()
	if err != nil {
		log.Fatal("Failed to initialize logger:", err)
	}
	defer zapLogger.Sync()

	zapLogger.Info("Starting Media Extraction Worker")

	// Configuration from environment
	redisAddr := getEnv("REDIS_ADDR", "localhost:6379")
	ytdlpPath := getEnv("YTDLP_PATH", "yt-dlp")
	ffmpegPath := getEnv("FFMPEG_PATH", "ffmpeg")
	s3Bucket := getEnv("S3_BUCKET", "media-extraction-output")
	s3Region := getEnv("S3_REGION", "us-east-1")
	s3Endpoint := getEnv("S3_ENDPOINT", "")

	// Disable localhost:9000 - not valid in production
	if s3Endpoint == "http://localhost:9000" || s3Endpoint == "localhost:9000" {
		s3Endpoint = ""
	}

	// Initialize extractors
	ytdlp := extractor.NewYtDlp(ytdlpPath, 10*time.Minute, zapLogger)
	ffmpeg := extractor.NewFFmpeg(ffmpegPath, 30*time.Minute, zapLogger)

	// Initialize storage
	// Use local file storage for now (S3 can be enabled via environment variables)
	var fileStorage interface {
		Upload(ctx context.Context, filePath, key string) error
		UploadStream(ctx context.Context, reader io.Reader, key string) error
		GetPresignedURL(ctx context.Context, key string) (string, error)
	}

	if s3Endpoint != "" && s3Endpoint != "disabled" {
		// Use S3 if endpoint is configured
		s3Stor, err := storage.NewS3Storage(context.Background(), storage.Config{
			Region:               s3Region,
			Bucket:               s3Bucket,
			Endpoint:             s3Endpoint,
			PresignedURLExpiry:   24 * time.Hour,
			StreamThresholdBytes: 500 * 1024 * 1024,
			Logger:               zapLogger,
		})
		if err != nil {
			zapLogger.Fatal("Failed to initialize S3 storage", zap.Error(err))
		}
		fileStorage = s3Stor
	} else {
		// Use local file storage (default)
		localStor, err := storage.NewLocalStorage("/app/downloads", zapLogger)
		if err != nil {
			zapLogger.Fatal("Failed to initialize local storage", zap.Error(err))
		}
		fileStorage = localStor
	}

	// Initialize queue client (for updating job status)
	queueClient := queue.NewClient(redisAddr, zapLogger)
	defer queueClient.Close()

	// Initialize extraction handler
	extractionHandler := handlers.NewExtractionHandler(
		ytdlp,
		ffmpeg,
		fileStorage,
		queueClient,
		zapLogger,
	)

	// Initialize queue server (worker)
	workerServer := queue.NewServer(queue.ServerConfig{
		RedisAddr:   redisAddr,
		Concurrency: getEnvInt("WORKER_CONCURRENCY", 8),
		Queues: map[string]int{
			"critical": 6, // 4K transcoding
			"default":  3, // 1080p
			"low":      1, // Audio-only
		},
		ShutdownTimeout: 30,
		Logger:          zapLogger,
		Handler:         extractionHandler,
	})

	zapLogger.Info("Worker configuration",
		zap.String("redis", redisAddr),
		zap.String("ytdlp", ytdlpPath),
		zap.String("ffmpeg", ffmpegPath),
		zap.String("s3_bucket", s3Bucket),
	)

	// Start worker in goroutine
	go func() {
		if err := workerServer.Start(); err != nil {
			zapLogger.Fatal("Worker error", zap.Error(err))
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	zapLogger.Info("Shutting down worker...")
	workerServer.Shutdown()

	zapLogger.Info("Worker stopped")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		// Simple int parsing (in production, handle errors properly)
		var result int
		if _, err := fmt.Sscanf(value, "%d", &result); err == nil {
			return result
		}
	}
	return defaultValue
}
