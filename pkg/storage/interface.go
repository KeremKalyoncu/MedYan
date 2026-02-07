package storage

import (
	"context"
	"io"
)

// Storage is the interface for file storage operations
type Storage interface {
	Upload(ctx context.Context, filePath, key string) error
	UploadStream(ctx context.Context, reader io.Reader, key string) error
	GetPresignedURL(ctx context.Context, key string) (string, error)
}
