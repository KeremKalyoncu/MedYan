package testutil

import (
	"context"
	"sync"

	"github.com/gsker/media-extraction-saas/internal/types"
	"go.uber.org/zap"
)

// MockStorage is a mock implementation of the Storage interface
type MockStorage struct {
	mu        sync.Mutex
	files     map[string][]byte
	urls      map[string]string
	shouldErr bool
}

// NewMockStorage creates a new mock storage
func NewMockStorage() *MockStorage {
	return &MockStorage{
		files: make(map[string][]byte),
		urls:  make(map[string]string),
	}
}

// Upload uploads a file to mock storage
func (m *MockStorage) Upload(ctx context.Context, key string, data []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldErr {
		return ErrMockStorageFailure
	}

	m.files[key] = data
	m.urls[key] = "http://localhost:9000/" + key
	return nil
}

// UploadFromFile uploads a file from a file path
func (m *MockStorage) UploadFromFile(ctx context.Context, key string, filePath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldErr {
		return ErrMockStorageFailure
	}

	// Simulate file upload
	m.files[key] = []byte("mock file content")
	m.urls[key] = "http://localhost:9000/" + key
	return nil
}

// Download downloads a file from mock storage
func (m *MockStorage) Download(ctx context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldErr {
		return nil, ErrMockStorageFailure
	}

	if data, ok := m.files[key]; ok {
		return data, nil
	}

	return nil, ErrMockFileNotFound
}

// GetPresignedURL returns a presigned URL for a file
func (m *MockStorage) GetPresignedURL(ctx context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.shouldErr {
		return "", ErrMockStorageFailure
	}

	if url, ok := m.urls[key]; ok {
		return url, nil
	}

	return "", ErrMockFileNotFound
}

// Delete deletes a file from mock storage
func (m *MockStorage) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.files, key)
	delete(m.urls, key)
	return nil
}

// Exists checks if a file exists in mock storage
func (m *MockStorage) Exists(ctx context.Context, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	_, exists := m.files[key]
	return exists, nil
}

// SetShouldError sets whether operations should return an error
func (m *MockStorage) SetShouldError(err bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.shouldErr = err
}

// GetFile returns a file from mock storage (for testing)
func (m *MockStorage) GetFile(key string) []byte {
	m.mu.Lock()
	defer m.mu.Unlock()

	if data, ok := m.files[key]; ok {
		return data
	}
	return nil
}

// FileCount returns the number of files in mock storage
func (m *MockStorage) FileCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.files)
}

// MockQueue is a mock implementation of the Queue interface
type MockQueue struct {
	mu    sync.Mutex
	jobs  map[string]*types.ExtractionJob
	queue []*types.ExtractionJob
}

// NewMockQueue creates a new mock queue
func NewMockQueue() *MockQueue {
	return &MockQueue{
		jobs:  make(map[string]*types.ExtractionJob),
		queue: make([]*types.ExtractionJob, 0),
	}
}

// Enqueue adds a job to the queue
func (m *MockQueue) Enqueue(job *types.ExtractionJob) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.jobs[job.ID] = job
	m.queue = append(m.queue, job)
	return nil
}

// Dequeue removes and returns the next job from the queue
func (m *MockQueue) Dequeue() *types.ExtractionJob {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.queue) == 0 {
		return nil
	}

	job := m.queue[0]
	m.queue = m.queue[1:]
	return job
}

// GetJob returns a job by ID
func (m *MockQueue) GetJob(id string) *types.ExtractionJob {
	m.mu.Lock()
	defer m.mu.Unlock()

	return m.jobs[id]
}

// UpdateJob updates a job
func (m *MockQueue) UpdateJob(job *types.ExtractionJob) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.jobs[job.ID] = job
	return nil
}

// Length returns the number of jobs in the queue
func (m *MockQueue) Length() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	return len(m.queue)
}

// Clear clears the queue
func (m *MockQueue) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.queue = make([]*types.ExtractionJob, 0)
	m.jobs = make(map[string]*types.ExtractionJob)
}

// Test fixtures and helpers
var (
	TestLogger, _ = zap.NewProduction()

	TestExtractionRequest = &types.ExtractionRequest{
		URL:     "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		Quality: "720p",
		Format:  "mp4",
	}

	TestMediaMetadata = &types.MediaMetadata{
		Title:      "Test Video",
		Duration:   100,
		Uploader:   "Test Channel",
		Platform:   "youtube",
		Width:      1280,
		Height:     720,
		FPS:        30,
		VideoCodec: "h264",
		AudioCodec: "aac",
	}

	TestExtractionResult = &types.ExtractionResult{
		DownloadURL: "http://localhost:9000/media-extraction-output/jobs/2025-01-01/test/video.mp4",
		Filename:    "video.mp4",
		SizeBytes:   20971520,
		Format:      "mp4",
	}
)

// Error variables for mock operations
var (
	ErrMockStorageFailure = &struct{ error }{error: nil}
	ErrMockFileNotFound   = &struct{ error }{error: nil}
)

// Helper function to create a test context
func TestContext() context.Context {
	return context.Background()
}

// Helper function to get test logger
func GetTestLogger() *zap.Logger {
	logger, _ := zap.NewProduction()
	return logger
}
