package queue

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hibiken/asynq"
	"go.uber.org/zap"

	"github.com/gsker/media-extraction-saas/internal/types"
)

// Server wraps Asynq server for job processing
type Server struct {
	asynq   *asynq.Server
	mux     *asynq.ServeMux
	logger  *zap.Logger
	handler JobHandler
}

// JobHandler defines the interface for processing extraction jobs
type JobHandler interface {
	HandleExtraction(ctx context.Context, job *types.ExtractionJob) error
}

// ServerConfig holds server configuration
type ServerConfig struct {
	RedisAddr      string
	Concurrency    int
	Queues         map[string]int
	ShutdownTimeout int // seconds
	Logger         *zap.Logger
	Handler        JobHandler
}

// NewServer creates a new queue server
func NewServer(cfg ServerConfig) *Server {
	asynqServer := asynq.NewServer(
		asynq.RedisClientOpt{Addr: cfg.RedisAddr},
		asynq.Config{
			Concurrency: cfg.Concurrency,
			Queues:      cfg.Queues,
			StrictPriority: false, // Fair distribution across queues
			Logger:      NewAsynqLogger(cfg.Logger),
			ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
				cfg.Logger.Error("Task failed",
					zap.String("type", task.Type()),
					zap.Error(err),
				)
			}),
		},
	)

	mux := asynq.NewServeMux()

	srv := &Server{
		asynq:   asynqServer,
		mux:     mux,
		logger:  cfg.Logger,
		handler: cfg.Handler,
	}

	// Register task handlers
	mux.HandleFunc(TypeExtraction, srv.handleExtractionTask)
	mux.HandleFunc(TypeBatch, srv.handleBatchTask)

	return srv
}

// Start starts the server
func (s *Server) Start() error {
	s.logger.Info("Starting worker server")
	return s.asynq.Run(s.mux)
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown() {
	s.logger.Info("Shutting down worker server")
	s.asynq.Shutdown()
}

// handleExtractionTask processes a media extraction task
func (s *Server) handleExtractionTask(ctx context.Context, task *asynq.Task) error {
	var job types.ExtractionJob
	if err := json.Unmarshal(task.Payload(), &job); err != nil {
		return fmt.Errorf("failed to unmarshal job: %w", err)
	}

	s.logger.Info("Processing extraction job",
		zap.String("job_id", job.ID),
		zap.String("url", job.Request.URL),
		zap.String("quality", job.Request.Quality),
	)

	// Update job status to processing
	job.Status = types.StatusProcessing
	job.Progress = 10

	// Delegate to handler
	if err := s.handler.HandleExtraction(ctx, &job); err != nil {
		job.Status = types.StatusFailed
		job.Error = err.Error()
		s.logger.Error("Extraction failed",
			zap.String("job_id", job.ID),
			zap.Error(err),
		)
		return err
	}

	s.logger.Info("Extraction completed",
		zap.String("job_id", job.ID),
	)

	return nil
}

// handleBatchTask processes a batch extraction task
func (s *Server) handleBatchTask(ctx context.Context, task *asynq.Task) error {
	// TODO: Implement batch processing logic
	s.logger.Info("Processing batch task")
	return nil
}

// AsynqLogger adapts zap.Logger to asynq.Logger interface
type AsynqLogger struct {
	logger *zap.Logger
}

func NewAsynqLogger(logger *zap.Logger) *AsynqLogger {
	return &AsynqLogger{logger: logger}
}

func (l *AsynqLogger) Debug(args ...interface{}) {
	l.logger.Debug(fmt.Sprint(args...))
}

func (l *AsynqLogger) Info(args ...interface{}) {
	l.logger.Info(fmt.Sprint(args...))
}

func (l *AsynqLogger) Warn(args ...interface{}) {
	l.logger.Warn(fmt.Sprint(args...))
}

func (l *AsynqLogger) Error(args ...interface{}) {
	l.logger.Error(fmt.Sprint(args...))
}

func (l *AsynqLogger) Fatal(args ...interface{}) {
	l.logger.Fatal(fmt.Sprint(args...))
}
