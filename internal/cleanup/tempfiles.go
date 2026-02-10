package cleanup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

// TempFileCleanup periodically removes old temporary files
// This prevents disk space issues from failed downloads
type TempFileCleanup struct {
	tempDir   string
	maxAge    time.Duration
	interval  time.Duration
	logger    *zap.Logger
	closeCh   chan struct{}
	stoppedCh chan struct{}
}

// NewTempFileCleanup creates a new temp file cleanup service
// maxAge: Files older than this are deleted (e.g., 1 hour)
// interval: How often to run cleanup (e.g., 30 minutes)
func NewTempFileCleanup(tempDir string, maxAge, interval time.Duration, logger *zap.Logger) *TempFileCleanup {
	return &TempFileCleanup{
		tempDir:   tempDir,
		maxAge:    maxAge,
		interval:  interval,
		logger:    logger,
		closeCh:   make(chan struct{}),
		stoppedCh: make(chan struct{}),
	}
}

// Start begins the cleanup goroutine
func (tfc *TempFileCleanup) Start(ctx context.Context) {
	go tfc.run(ctx)
}

// Stop stops the cleanup goroutine
func (tfc *TempFileCleanup) Stop() {
	close(tfc.closeCh)
	<-tfc.stoppedCh // Wait for cleanup to finish
}

func (tfc *TempFileCleanup) run(ctx context.Context) {
	defer close(tfc.stoppedCh)

	ticker := time.NewTicker(tfc.interval)
	defer ticker.Stop()

	// Run once at startup
	tfc.cleanup(ctx)

	for {
		select {
		case <-ticker.C:
			tfc.cleanup(ctx)
		case <-tfc.closeCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (tfc *TempFileCleanup) cleanup(ctx context.Context) {
	now := time.Now()
	cutoff := now.Add(-tfc.maxAge)

	var deletedCount int
	var deletedSize int64
	var errorCount int

	tfc.logger.Info("Starting temp file cleanup",
		zap.String("dir", tfc.tempDir),
		zap.Duration("max_age", tfc.maxAge),
	)

	// Walk temp directory
	err := filepath.Walk(tfc.tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip files we can't access
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Check if file is old enough to delete
		if info.ModTime().Before(cutoff) {
			// Check for our temp file patterns (optional: add pattern matching)
			// e.g., files starting with "ytdlp-", "medyan-", etc.

			size := info.Size()
			if err := os.Remove(path); err != nil {
				tfc.logger.Warn("Failed to delete old temp file",
					zap.String("file", path),
					zap.Duration("age", now.Sub(info.ModTime())),
					zap.Error(err),
				)
				errorCount++
			} else {
				tfc.logger.Debug("Deleted old temp file",
					zap.String("file", path),
					zap.Duration("age", now.Sub(info.ModTime())),
					zap.Int64("size", size),
				)
				deletedCount++
				deletedSize += size
			}
		}

		return nil
	})

	if err != nil {
		tfc.logger.Error("Temp file cleanup failed",
			zap.Error(err),
		)
		return
	}

	tfc.logger.Info("Temp file cleanup completed",
		zap.Int("deleted_count", deletedCount),
		zap.Int64("freed_bytes", deletedSize),
		zap.String("freed_mb", formatBytes(deletedSize)),
		zap.Int("errors", errorCount),
	)
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return "0 MB"
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
