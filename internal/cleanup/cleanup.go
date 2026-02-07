package cleanup

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

// Manager handles file cleanup operations
type Manager struct {
	logger *zap.Logger
}

// NewManager creates a new cleanup manager
func NewManager(logger *zap.Logger) *Manager {
	return &Manager{
		logger: logger,
	}
}

// CleanupOptions configures cleanup behavior
type CleanupOptions struct {
	// Age of files to delete (delete older than this)
	DeleteOlderThan time.Duration

	// Maximum directory size before cleanup (in bytes)
	MaxDirectorySize int64

	// Dry run - don't actually delete, just report
	DryRun bool

	// Recursively clean subdirectories
	Recursive bool
}

// CleanupResult contains cleanup operation results
type CleanupResult struct {
	FilesDeleted     int64   // Number of files deleted
	BytesFreed       int64   // Number of bytes freed
	DirectoryCleaned string  // Path to cleaned directory
	Errors           []error // Any errors that occurred
}

// CleanTempFiles removes temporary files from extraction jobs
func (m *Manager) CleanTempFiles(tempDir string, opts CleanupOptions) *CleanupResult {
	result := &CleanupResult{
		DirectoryCleaned: tempDir,
		Errors:           make([]error, 0),
	}

	// Ensure directory exists
	if _, err := os.Stat(tempDir); os.IsNotExist(err) {
		m.logger.Warn("Temp directory does not exist", zap.String("path", tempDir))
		return result
	}

	// Walk directory
	err := filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("error accessing %s: %w", path, err))
			return nil
		}

		// Skip directories if not recursive
		if info.IsDir() {
			if !opts.Recursive && path != tempDir {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if file is old enough
		age := time.Since(info.ModTime())
		if age > opts.DeleteOlderThan {
			// Delete file
			if !opts.DryRun {
				if err := os.Remove(path); err != nil {
					result.Errors = append(result.Errors, fmt.Errorf("failed to delete %s: %w", path, err))
					return nil
				}
			}

			result.FilesDeleted++
			result.BytesFreed += info.Size()

			m.logger.Debug("Cleaned temp file",
				zap.String("file", path),
				zap.Int64("size", info.Size()),
				zap.Duration("age", age),
			)
		}

		return nil
	})

	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("walk error: %w", err))
	}

	m.logger.Info("Cleanup completed",
		zap.String("dir", tempDir),
		zap.Int64("files_deleted", result.FilesDeleted),
		zap.Int64("bytes_freed", result.BytesFreed),
		zap.Int("errors", len(result.Errors)),
	)

	return result
}

// CleanLogFiles removes old log files
func (m *Manager) CleanLogFiles(logDir string, opts CleanupOptions) *CleanupResult {
	result := &CleanupResult{
		DirectoryCleaned: logDir,
		Errors:           make([]error, 0),
	}

	// Ensure directory exists
	if _, err := os.Stat(logDir); os.IsNotExist(err) {
		m.logger.Warn("Log directory does not exist", zap.String("path", logDir))
		return result
	}

	// Walk directory
	err := filepath.Walk(logDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("error accessing %s: %w", path, err))
			return nil
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Only clean .log files
		if filepath.Ext(path) != ".log" {
			return nil
		}

		// Check if file is old enough
		age := time.Since(info.ModTime())
		if age > opts.DeleteOlderThan {
			if !opts.DryRun {
				if err := os.Remove(path); err != nil {
					result.Errors = append(result.Errors, fmt.Errorf("failed to delete %s: %w", path, err))
					return nil
				}
			}

			result.FilesDeleted++
			result.BytesFreed += info.Size()

			m.logger.Debug("Cleaned log file",
				zap.String("file", path),
				zap.Int64("size", info.Size()),
				zap.Duration("age", age),
			)
		}

		return nil
	})

	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("walk error: %w", err))
	}

	m.logger.Info("Log cleanup completed",
		zap.String("dir", logDir),
		zap.Int64("files_deleted", result.FilesDeleted),
		zap.Int64("bytes_freed", result.BytesFreed),
	)

	return result
}

// RemoveDirectory removes an entire directory and its contents
func (m *Manager) RemoveDirectory(dirPath string, dryRun bool) error {
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		m.logger.Warn("Directory does not exist", zap.String("path", dirPath))
		return nil
	}

	if !dryRun {
		if err := os.RemoveAll(dirPath); err != nil {
			m.logger.Error("Failed to remove directory",
				zap.String("path", dirPath),
				zap.Error(err),
			)
			return fmt.Errorf("failed to remove directory: %w", err)
		}
	}

	m.logger.Info("Directory removed",
		zap.String("path", dirPath),
		zap.Bool("dry_run", dryRun),
	)

	return nil
}

// GetDirectorySize calculates total directory size in bytes
func (m *Manager) GetDirectorySize(dirPath string) (int64, error) {
	var totalSize int64

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			totalSize += info.Size()
		}
		return nil
	})

	return totalSize, err
}

// GetDirectoryStats returns detailed directory statistics
type DirectoryStats struct {
	TotalSize   int64
	FileCount   int64
	OldestFile  time.Time
	NewestFile  time.Time
	AverageSize int64
}

// GetDirectoryStats calculates directory statistics
func (m *Manager) GetDirectoryStats(dirPath string) (*DirectoryStats, error) {
	stats := &DirectoryStats{
		OldestFile: time.Now(),
	}

	var firstFile = true

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() {
			stats.TotalSize += info.Size()
			stats.FileCount++

			modTime := info.ModTime()
			if modTime.Before(stats.OldestFile) {
				stats.OldestFile = modTime
			}

			if firstFile || modTime.After(stats.NewestFile) {
				stats.NewestFile = modTime
				firstFile = false
			}
		}

		return nil
	})

	if stats.FileCount > 0 {
		stats.AverageSize = stats.TotalSize / stats.FileCount
	}

	return stats, err
}

// CleanupStrategy defines automatic cleanup behavior
type CleanupStrategy struct {
	// Enable automatic cleanup
	Enabled bool

	// Run cleanup every N duration
	Interval time.Duration

	// Delete files older than this
	MaxAge time.Duration

	// Maximum size before cleanup is forced
	MaxSize int64

	// Directories to clean
	Directories []string
}

// Worker performs scheduled cleanup operations
type Worker struct {
	strategy *CleanupStrategy
	manager  *Manager
	logger   *zap.Logger
	stopCh   chan struct{}
}

// NewWorker creates a new cleanup worker
func NewWorker(strategy *CleanupStrategy, manager *Manager, logger *zap.Logger) *Worker {
	return &Worker{
		strategy: strategy,
		manager:  manager,
		logger:   logger,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the periodic cleanup process
func (w *Worker) Start() {
	if !w.strategy.Enabled {
		w.logger.Info("Cleanup worker is disabled")
		return
	}

	go func() {
		ticker := time.NewTicker(w.strategy.Interval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				w.performCleanup()
			case <-w.stopCh:
				w.logger.Info("Cleanup worker stopped")
				return
			}
		}
	}()

	w.logger.Info("Cleanup worker started",
		zap.Duration("interval", w.strategy.Interval),
		zap.Duration("max_age", w.strategy.MaxAge),
	)
}

// Stop stops the cleanup worker
func (w *Worker) Stop() {
	close(w.stopCh)
}

// performCleanup runs cleanup on all configured directories
func (w *Worker) performCleanup() {
	w.logger.Info("Starting cleanup cycle")

	opts := CleanupOptions{
		DeleteOlderThan: w.strategy.MaxAge,
		Recursive:       true,
	}

	for _, dir := range w.strategy.Directories {
		result := w.manager.CleanTempFiles(dir, opts)

		if len(result.Errors) > 0 {
			w.logger.Error("Cleanup errors occurred",
				zap.String("dir", dir),
				zap.Int("error_count", len(result.Errors)),
			)
			for _, err := range result.Errors {
				w.logger.Error("Cleanup error", zap.Error(err))
			}
		}
	}

	w.logger.Info("Cleanup cycle completed")
}
