package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/gsker/media-extraction-saas/internal/extractor"
	"github.com/gsker/media-extraction-saas/internal/queue"
	"github.com/gsker/media-extraction-saas/internal/types"
	"github.com/gsker/media-extraction-saas/pkg/storage"
)

// ExtractionHandler handles media extraction jobs
type ExtractionHandler struct {
	ytdlp   *extractor.YtDlp
	ffmpeg  *extractor.FFmpeg
	storage *storage.S3Storage
	queue   *queue.Client
	logger  *zap.Logger
	tempDir string
}

// NewExtractionHandler creates a new extraction handler
func NewExtractionHandler(
	ytdlp *extractor.YtDlp,
	ffmpeg *extractor.FFmpeg,
	s3Storage *storage.S3Storage,
	queueClient *queue.Client,
	logger *zap.Logger,
) *ExtractionHandler {
	return &ExtractionHandler{
		ytdlp:   ytdlp,
		ffmpeg:  ffmpeg,
		storage: s3Storage,
		queue:   queueClient,
		logger:  logger,
		tempDir: os.TempDir(),
	}
}

// HandleExtraction processes a media extraction job
func (h *ExtractionHandler) HandleExtraction(ctx context.Context, job *types.ExtractionJob) error {
	h.logger.Info("Starting extraction",
		zap.String("job_id", job.ID),
		zap.String("url", job.Request.URL),
	)

	// Update status to processing
	if err := h.queue.UpdateJobStatus(ctx, job.ID, types.StatusProcessing, 10, ""); err != nil {
		return err
	}

	// Step 1: Extract metadata
	metadata, err := h.ytdlp.ExtractMetadata(ctx, job.Request.URL)
	if err != nil {
		return h.handleError(ctx, job.ID, fmt.Errorf("metadata extraction failed: %w", err))
	}

	job.Metadata = metadata

	// Update progress
	if err := h.queue.UpdateJobStatus(ctx, job.ID, types.StatusProcessing, 30, ""); err != nil {
		return err
	}

	// Step 2: Download media
	downloadedFile, err := h.downloadMedia(ctx, job)
	if err != nil {
		return h.handleError(ctx, job.ID, fmt.Errorf("download failed: %w", err))
	}
	defer storage.CleanupTempFile(downloadedFile, h.logger)

	// Update progress
	if err := h.queue.UpdateJobStatus(ctx, job.ID, types.StatusProcessing, 70, ""); err != nil {
		return err
	}

	// Step 3: Post-process if needed (format conversion, quality adjustment)
	processedFile := downloadedFile
	if job.Request.Format != "" || job.Request.Quality != "" {
		processedFile, err = h.postProcess(ctx, job, downloadedFile)
		if err != nil {
			return h.handleError(ctx, job.ID, fmt.Errorf("post-processing failed: %w", err))
		}
		if processedFile != downloadedFile {
			defer storage.CleanupTempFile(processedFile, h.logger)
		}
	}

	// Update progress
	if err := h.queue.UpdateJobStatus(ctx, job.ID, types.StatusProcessing, 85, ""); err != nil {
		return err
	}

	// Step 4: Upload to S3
	result, err := h.uploadResult(ctx, job, processedFile)
	if err != nil {
		return h.handleError(ctx, job.ID, fmt.Errorf("upload failed: %w", err))
	}

	// Step 5: Mark as completed
	if err := h.queue.UpdateJobResult(ctx, job.ID, result, metadata); err != nil {
		return err
	}

	h.logger.Info("Extraction completed successfully",
		zap.String("job_id", job.ID),
	)

	return nil
}

// downloadMedia downloads the media file using yt-dlp
func (h *ExtractionHandler) downloadMedia(ctx context.Context, job *types.ExtractionJob) (string, error) {
	// Generate temp file path
	outputPath := filepath.Join(h.tempDir, fmt.Sprintf("%s.%%(ext)s", job.ID))

	// Prepare download options
	opts := extractor.DownloadOptions{
		Quality:      job.Request.Quality,
		Format:       job.Request.Format,
		ExtractAudio: job.Request.ExtractAudio,
		AudioFormat:  job.Request.AudioFormat,
		AudioBitrate: job.Request.AudioBitrate,
		Subtitles:    job.Request.Subtitles,
		UserAgent:    job.Request.UserAgent,
		ProxyURL:     job.Request.ProxyURL,
		ProgressCallback: func(progress int) {
			// Update progress: 30-70% range for download
			adjustedProgress := 30 + int(float64(progress)*0.4)
			h.queue.UpdateJobStatus(ctx, job.ID, types.StatusProcessing, adjustedProgress, "")
		},
	}

	// Handle cookies if provided
	if job.Request.CookiesBase64 != "" {
		cookieFile, err := extractor.WriteCookiesFile(job.Request.CookiesBase64)
		if err != nil {
			return "", err
		}
		opts.CookiesFile = cookieFile
		defer os.Remove(cookieFile)
	}

	// Download
	_, err := h.ytdlp.Download(ctx, job.Request.URL, outputPath, opts)
	if err != nil {
		return "", err
	}

	// Find the actual downloaded file (yt-dlp replaces %(ext)s)
	actualFile := h.findDownloadedFile(outputPath, job.ID)
	if actualFile == "" {
		return "", fmt.Errorf("downloaded file not found")
	}

	return actualFile, nil
}

// postProcess applies additional processing (format conversion, quality adjustment)
func (h *ExtractionHandler) postProcess(ctx context.Context, job *types.ExtractionJob, inputFile string) (string, error) {
	// If audio extraction was already done by yt-dlp, skip
	if job.Request.ExtractAudio {
		return inputFile, nil
	}

	// Format conversion if requested
	if job.Request.Format != "" {
		outputFile, err := h.ffmpeg.ConvertFormat(ctx, inputFile, job.Request.Format, "libx264", "")
		if err != nil {
			return "", err
		}
		return outputFile, nil
	}

	// Quality downscaling is handled by yt-dlp during download
	// (yt-dlp downloads at the requested quality, no post-processing needed)

	return inputFile, nil
}

// uploadResult uploads the processed file to S3 and generates presigned URL
func (h *ExtractionHandler) uploadResult(ctx context.Context, job *types.ExtractionJob, filePath string) (*types.ExtractionResult, error) {
	// Get file info
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return nil, err
	}

	// Generate S3 key
	filename := filepath.Base(filePath)
	key := storage.GenerateKey(job.ID, filename)

	// Upload
	if err := h.storage.Upload(ctx, filePath, key); err != nil {
		return nil, err
	}

	// Generate presigned URL
	downloadURL, err := h.storage.GetPresignedURL(ctx, key)
	if err != nil {
		return nil, err
	}

	result := &types.ExtractionResult{
		DownloadURL: downloadURL,
		Filename:    filename,
		SizeBytes:   fileInfo.Size(),
		Format:      filepath.Ext(filename),
		ExpiresAt:   time.Now().Add(24 * time.Hour),
	}

	return result, nil
}

// handleError updates job status with error and logs
func (h *ExtractionHandler) handleError(ctx context.Context, jobID string, err error) error {
	h.logger.Error("Extraction error",
		zap.String("job_id", jobID),
		zap.Error(err),
	)

	if updateErr := h.queue.UpdateJobStatus(ctx, jobID, types.StatusFailed, 0, err.Error()); updateErr != nil {
		h.logger.Error("Failed to update job status",
			zap.Error(updateErr),
		)
	}

	return err
}

// findDownloadedFile locates the downloaded file
func (h *ExtractionHandler) findDownloadedFile(outputTemplate, jobID string) string {
	// yt-dlp replaces %(ext)s with actual extension
	baseDir := filepath.Dir(outputTemplate)

	// Prefer a merged, clean output name when present.
	preferred := []string{
		filepath.Join(baseDir, jobID+".mp4"),
		filepath.Join(baseDir, jobID+".mkv"),
		filepath.Join(baseDir, jobID+".webm"),
		filepath.Join(baseDir, jobID+".mp3"),
		filepath.Join(baseDir, jobID+".m4a"),
	}
	for _, p := range preferred {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}

	matches, err := filepath.Glob(filepath.Join(baseDir, jobID+".*"))
	if err != nil || len(matches) == 0 {
		return ""
	}

	// Otherwise, pick the largest plausible media file.
	var bestPath string
	var bestSize int64 = -1
	for _, candidate := range matches {
		name := filepath.Base(candidate)
		ext := strings.ToLower(filepath.Ext(name))

		// Skip temporary/partial files and sidecars.
		if strings.HasSuffix(strings.ToLower(name), ".part") || strings.HasSuffix(strings.ToLower(name), ".ytdl") {
			continue
		}
		switch ext {
		case ".json", ".srt", ".vtt", ".ass", ".lrc":
			continue
		}

		info, statErr := os.Stat(candidate)
		if statErr != nil || info.IsDir() {
			continue
		}
		if info.Size() > bestSize {
			bestSize = info.Size()
			bestPath = candidate
		}
	}

	return bestPath
}
