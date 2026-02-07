package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"go.uber.org/zap"
)

// S3Storage handles file uploads to S3-compatible storage
type S3Storage struct {
	client               *s3.Client
	bucket               string
	endpoint             string // MinIO/R2 endpoint for public URL generation
	presignedURLExpiry   time.Duration
	streamThresholdBytes int64
	logger               *zap.Logger
}

// Config holds S3 storage configuration
type Config struct {
	Region               string
	Bucket               string
	Endpoint             string // For R2/MinIO
	AccessKey            string
	SecretKey            string
	PresignedURLExpiry   time.Duration
	StreamThresholdBytes int64 // Files <threshold use diskless streaming
	Logger               *zap.Logger
}

// NewS3Storage creates a new S3 storage client
func NewS3Storage(ctx context.Context, cfg Config) (*S3Storage, error) {
	// Load AWS config
	awsCfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(cfg.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client with optional custom endpoint
	clientOpts := []func(*s3.Options){}

	if cfg.Endpoint != "" {
		clientOpts = append(clientOpts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(cfg.Endpoint)
			o.UsePathStyle = true // Required for MinIO
		})
	}

	client := s3.NewFromConfig(awsCfg, clientOpts...)

	return &S3Storage{
		client:               client,
		bucket:               cfg.Bucket,
		endpoint:             cfg.Endpoint,
		presignedURLExpiry:   cfg.PresignedURLExpiry,
		streamThresholdBytes: cfg.StreamThresholdBytes,
		logger:               cfg.Logger,
	}, nil
}

// Upload uploads a file to S3
func (s *S3Storage) Upload(ctx context.Context, filePath, key string) error {
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}

	fileSize := fileInfo.Size()

	s.logger.Info("Uploading file to S3",
		zap.String("file", filePath),
		zap.String("key", key),
		zap.Int64("size", fileSize),
	)

	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Use multipart upload for large files
	if fileSize > 100*1024*1024 { // 100MB
		return s.multipartUpload(ctx, file, key, fileSize)
	}

	// Standard upload for smaller files
	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   file,
	})

	if err != nil {
		return fmt.Errorf("failed to upload to S3: %w", err)
	}

	s.logger.Info("Upload completed",
		zap.String("key", key),
	)

	return nil
}

// UploadStream uploads data from a reader (diskless streaming)
func (s *S3Storage) UploadStream(ctx context.Context, reader io.Reader, key string) error {
	s.logger.Info("Streaming upload to S3",
		zap.String("key", key),
	)

	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   reader,
	})

	if err != nil {
		return fmt.Errorf("failed to stream to S3: %w", err)
	}

	s.logger.Info("Stream upload completed",
		zap.String("key", key),
	)

	return nil
}

// multipartUpload performs multipart upload for large files
func (s *S3Storage) multipartUpload(ctx context.Context, file *os.File, key string, fileSize int64) error {
	s.logger.Info("Starting multipart upload",
		zap.String("key", key),
		zap.Int64("size", fileSize),
	)

	// This is a simplified version
	// In production, implement proper multipart upload with part tracking
	// and retry logic

	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
		Body:   file,
	})

	return err
}

// GetPresignedURL generates a presigned URL for downloading
func (s *S3Storage) GetPresignedURL(ctx context.Context, key string) (string, error) {
	// For public buckets (MinIO/R2 with public access), return direct URL
	if s.endpoint != "" {
		publicURL := fmt.Sprintf("%s/%s/%s", s.endpoint, s.bucket, key)
		s.logger.Info("Generated public URL",
			zap.String("key", key),
			zap.String("url", publicURL),
		)
		return publicURL, nil
	}

	// For AWS S3, use presigned URLs
	presignClient := s3.NewPresignClient(s.client)

	req, err := presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = s.presignedURLExpiry
	})

	if err != nil {
		return "", fmt.Errorf("failed to generate presigned URL: %w", err)
	}

	s.logger.Info("Generated presigned URL",
		zap.String("key", key),
		zap.Duration("expires_in", s.presignedURLExpiry),
	)

	return req.URL, nil
}

// Delete deletes a file from S3
func (s *S3Storage) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})

	if err != nil {
		return fmt.Errorf("failed to delete from S3: %w", err)
	}

	s.logger.Info("File deleted",
		zap.String("key", key),
	)

	return nil
}

// GenerateKey creates a unique S3 key for a file
func GenerateKey(jobID, filename string) string {
	// Structure: jobs/{date}/{job_id}/{filename}
	date := time.Now().Format("2006-01-02")
	// Use URL-style separators for S3 keys (always '/')
	return path.Join("jobs", date, jobID, filename)
}

// CleanupTempFile deletes a temporary file
func CleanupTempFile(filePath string, logger *zap.Logger) {
	if err := os.Remove(filePath); err != nil {
		logger.Error("Failed to cleanup temp file",
			zap.String("file", filePath),
			zap.Error(err),
		)
	} else {
		logger.Debug("Temp file cleaned up",
			zap.String("file", filePath),
		)
	}
}
