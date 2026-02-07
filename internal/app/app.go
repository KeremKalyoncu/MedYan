package app

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/KeremKalyoncu/MedYan/internal/config"
	"github.com/KeremKalyoncu/MedYan/internal/extractor"
	"github.com/KeremKalyoncu/MedYan/internal/handlers"
	"github.com/KeremKalyoncu/MedYan/internal/queue"
	"github.com/KeremKalyoncu/MedYan/pkg/storage"
)

// Container holds all application dependencies
type Container struct {
	Config            *config.Config
	Logger            *zap.Logger
	QueueClient       *queue.Client
	ExtractorYtdlp    *extractor.YtDlp
	ExtractorFFmpeg   *extractor.FFmpeg
	Storage           storage.Storage
	ExtractionHandler *handlers.ExtractionHandler
	WorkerServer      *queue.Server
}

// NewContainer creates and initializes a new application container
func NewContainer(logger *zap.Logger) (*Container, error) {
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	logger.Info("Configuration loaded successfully",
		zap.String("redis_addr", cfg.Redis.Address),
		zap.String("s3_endpoint", cfg.Storage.Endpoint),
		zap.String("api_port", fmt.Sprintf("%d", cfg.API.Port)),
	)

	// Initialize queue client
	queueClient := queue.NewClient(cfg.Redis.Address, logger)

	// Initialize extractors
	ytdlp := extractor.NewYtDlp(
		cfg.Extractor.YtdlpPath,
		cfg.Extractor.YtdlpTimeout,
		logger,
	)

	ffmpegExtractor := extractor.NewFFmpeg(
		cfg.Extractor.FFmpegPath,
		cfg.Extractor.FFmpegTimeout,
		logger,
	)

	// Initialize storage
	s3Storage, err := storage.NewS3Storage(
		context.Background(),
		storage.Config{
			Region:               cfg.Storage.Region,
			Bucket:               cfg.Storage.Bucket,
			Endpoint:             cfg.Storage.Endpoint,
			PresignedURLExpiry:   cfg.Storage.PresignedURLExpiry,
			StreamThresholdBytes: cfg.Storage.StreamThresholdBytes,
			Logger:               logger,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize S3 storage: %w", err)
	}

	logger.Info("Storage initialized successfully",
		zap.String("bucket", cfg.Storage.Bucket),
		zap.String("endpoint", cfg.Storage.Endpoint),
	)

	// Initialize extraction handler
	extractionHandler := handlers.NewExtractionHandler(
		ytdlp,
		ffmpegExtractor,
		s3Storage,
		queueClient,
		logger,
	)

	// Initialize worker server
	workerServer := queue.NewServer(queue.ServerConfig{
		RedisAddr:   cfg.Redis.Address,
		Concurrency: cfg.Worker.Concurrency,
		Queues: map[string]int{
			"critical": 6, // 4K transcoding
			"default":  3, // Standard quality
			"low":      1, // Audio-only / low priority
		},
		ShutdownTimeout: int(cfg.Worker.ShutdownTimeout / time.Second),
		Logger:          logger,
		Handler:         extractionHandler,
	})

	return &Container{
		Config:            cfg,
		Logger:            logger,
		QueueClient:       queueClient,
		ExtractorYtdlp:    ytdlp,
		ExtractorFFmpeg:   ffmpegExtractor,
		Storage:           s3Storage,
		ExtractionHandler: extractionHandler,
		WorkerServer:      workerServer,
	}, nil
}

// Close closes all resources
func (c *Container) Close() error {
	c.Logger.Info("Closing application container")

	if c.QueueClient != nil {
		c.QueueClient.Close()
	}

	return nil
}
