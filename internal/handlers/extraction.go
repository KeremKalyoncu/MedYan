package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/KeremKalyoncu/MedYan/internal/extractor"
	"github.com/KeremKalyoncu/MedYan/internal/metrics"
	"github.com/KeremKalyoncu/MedYan/internal/queue"
	"github.com/KeremKalyoncu/MedYan/internal/types"
	"github.com/KeremKalyoncu/MedYan/pkg/storage"
)

// ExtractionHandler handles media extraction jobs
type ExtractionHandler struct {
	ytdlp   *extractor.YtDlp
	ffmpeg  *extractor.FFmpeg
	storage storage.Storage
	queue   *queue.Client
	logger  *zap.Logger
	tempDir string
}

// NewExtractionHandler creates a new extraction handler
func NewExtractionHandler(
	ytdlp *extractor.YtDlp,
	ffmpeg *extractor.FFmpeg,
	s3Storage storage.Storage,
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
	startTime := time.Now()
	metricsInstance := metrics.GetMetrics()

	// Detect platform from URL
	platform := h.detectPlatform(job.Request.URL)
	metricsInstance.RecordJobStart(platform)

	h.logger.Info("Starting extraction",
		zap.String("job_id", job.ID),
		zap.String("url", job.Request.URL),
		zap.String("platform", platform),
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

	// Record metrics
	duration := time.Since(startTime)
	sizeMB := uint64(result.SizeBytes / (1024 * 1024))
	metricsInstance.RecordJobSuccess(platform, duration, sizeMB)

	h.logger.Info("Extraction completed successfully",
		zap.String("job_id", job.ID),
		zap.Duration("duration", duration),
		zap.Uint64("size_mb", sizeMB),
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
		h.logger.Info("Converting format",
			zap.String("job_id", job.ID),
			zap.String("input", inputFile),
			zap.String("target_format", job.Request.Format),
		)

		// Use copy codec if possible (no re-encoding, just remux)
		// This is 100x faster and uses minimal memory
		codec := "copy"

		// Only re-encode if format change requires it
		inputExt := strings.ToLower(filepath.Ext(inputFile))
		targetExt := strings.ToLower(job.Request.Format)
		if !strings.HasPrefix(targetExt, ".") {
			targetExt = "." + targetExt
		}

		// If extensions are different and codecs are incompatible, re-encode
		// But use fast, memory-efficient codec
		if inputExt != targetExt {
			// Check if we can just remux (container change only)
			if canRemux(inputExt, targetExt) {
				codec = "copy" // Just remux, no re-encoding
			} else {
				codec = "libx264" // Re-encode only if necessary
			}
		}

		outputFile, err := h.ffmpeg.ConvertFormat(ctx, inputFile, job.Request.Format, codec, "")
		if err != nil {
			return "", fmt.Errorf("format conversion failed: %w", err)
		}

		h.logger.Info("Format conversion completed",
			zap.String("job_id", job.ID),
			zap.String("output", outputFile),
			zap.String("codec", codec),
		)

		return outputFile, nil
	}

	// Quality downscaling is handled by yt-dlp during download
	// (yt-dlp downloads at the requested quality, no post-processing needed)

	return inputFile, nil
}

// canRemux checks if we can remux (container change) without re-encoding
func canRemux(inputExt, targetExt string) bool {
	// MP4, MKV, AVI can usually remux between each other
	remuxable := map[string][]string{
		".mp4":  {".mkv", ".avi", ".mov"},
		".mkv":  {".mp4", ".avi", ".mov"},
		".avi":  {".mp4", ".mkv"},
		".mov":  {".mp4", ".mkv"},
		".webm": {".mkv"},
	}

	if targets, ok := remuxable[inputExt]; ok {
		for _, t := range targets {
			if t == targetExt {
				return true
			}
		}
	}

	return false
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

	// Record failure metric (platform unknown here, could be improved)
	metricsInstance := metrics.GetMetrics()
	metricsInstance.RecordJobFailure("unknown")

	if updateErr := h.queue.UpdateJobStatus(ctx, jobID, types.StatusFailed, 0, err.Error()); updateErr != nil {
		h.logger.Error("Failed to update job status",
			zap.Error(updateErr),
		)
	}

	return err
}

// detectPlatform detects platform from URL
func (h *ExtractionHandler) detectPlatform(url string) string {
	url = strings.ToLower(url)

	if strings.Contains(url, "youtube.com") || strings.Contains(url, "youtu.be") {
		return "youtube"
	}
	if strings.Contains(url, "instagram.com") {
		return "instagram"
	}
	if strings.Contains(url, "tiktok.com") {
		return "tiktok"
	}
	if strings.Contains(url, "twitter.com") || strings.Contains(url, "x.com") {
		return "twitter"
	}
	if strings.Contains(url, "facebook.com") || strings.Contains(url, "fb.watch") {
		return "facebook"
	}
	if strings.Contains(url, "vimeo.com") {
		return "vimeo"
	}

	return "other"
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
