package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"go.uber.org/zap"
)

// LocalStorage handles file uploads to local filesystem
type LocalStorage struct {
	basePath string
	logger   *zap.Logger
}

// NewLocalStorage creates a new local storage handler
func NewLocalStorage(basePath string, logger *zap.Logger) (*LocalStorage, error) {
	// Create base directory if it doesn't exist
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}

	return &LocalStorage{
		basePath: basePath,
		logger:   logger,
	}, nil
}

// Upload uploads a file to local storage
func (ls *LocalStorage) Upload(ctx context.Context, filePath, key string) error {
	// Create subdirectories if needed
	fullPath := filepath.Join(ls.basePath, key)
	dir := filepath.Dir(fullPath)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Copy file
	src, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer src.Close()

	dst, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}

	ls.logger.Info("File uploaded to local storage",
		zap.String("key", key),
		zap.String("path", fullPath),
	)

	return nil
}

// UploadStream uploads from a reader to local storage
func (ls *LocalStorage) UploadStream(ctx context.Context, reader io.Reader, key string) error {
	fullPath := filepath.Join(ls.basePath, key)
	dir := filepath.Dir(fullPath)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := os.Create(fullPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, reader); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	ls.logger.Info("Stream uploaded to local storage", zap.String("key", key))
	return nil
}

// GetPresignedURL returns a URL to download the file
// For local storage, this is a relative path
func (ls *LocalStorage) GetPresignedURL(ctx context.Context, key string) (string, error) {
	// Return URL path relative to downloads folder
	// Frontend will need to access it as /downloads/{key}
	return "/downloads/" + key, nil
}

// GetFile returns file content for direct download
func (ls *LocalStorage) GetFile(ctx context.Context, key string) (*os.File, error) {
	fullPath := filepath.Join(ls.basePath, key)
	return os.Open(fullPath)
}

// Delete removes a file from local storage
func (ls *LocalStorage) Delete(ctx context.Context, key string) error {
	fullPath := filepath.Join(ls.basePath, key)
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete file: %w", err)
	}
	return nil
}

// Cleanup removes old files (older than maxAge)
func (ls *LocalStorage) Cleanup(ctx context.Context, maxAge time.Duration) error {
	return filepath.Walk(ls.basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		if time.Since(info.ModTime()) > maxAge {
			if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
				ls.logger.Error("Failed to delete old file", zap.Error(err), zap.String("path", path))
			}
		}

		return nil
	})
}
