package shutdown

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// GracefulShutdown handles graceful application shutdown
type GracefulShutdown struct {
	logger   *zap.Logger
	timeout  time.Duration
	handlers []func(ctx context.Context) error
}

// NewGracefulShutdown creates a shutdown handler
func NewGracefulShutdown(logger *zap.Logger, timeout time.Duration) *GracefulShutdown {
	return &GracefulShutdown{
		logger:   logger,
		timeout:  timeout,
		handlers: []func(ctx context.Context) error{},
	}
}

// Register adds a cleanup handler
func (gs *GracefulShutdown) Register(handler func(ctx context.Context) error) {
	gs.handlers = append(gs.handlers, handler)
}

// Wait blocks until shutdown signal is received
func (gs *GracefulShutdown) Wait() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)

	sig := <-quit
	gs.logger.Info("Shutdown signal received", zap.String("signal", sig.String()))

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), gs.timeout)
	defer cancel()

	// Execute all cleanup handlers
	for i, handler := range gs.handlers {
		gs.logger.Info("Executing cleanup handler", zap.Int("handler", i+1))

		if err := handler(ctx); err != nil {
			gs.logger.Error("Cleanup handler failed",
				zap.Int("handler", i+1),
				zap.Error(err),
			)
		}
	}

	gs.logger.Info("Graceful shutdown completed")
}
