package extractor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/KeremKalyoncu/MedYan/internal/circuitbreaker"
	"github.com/KeremKalyoncu/MedYan/internal/retry"
	"github.com/KeremKalyoncu/MedYan/internal/types"
)

// YtDlp wraps yt-dlp for media extraction with resilience patterns
type YtDlp struct {
	binaryPath     string
	timeout        time.Duration
	logger         *zap.Logger
	circuitBreaker *circuitbreaker.CircuitBreaker
	retryConfig    retry.Config
}

// NewYtDlp creates a new yt-dlp wrapper with circuit breaker and retry logic
func NewYtDlp(binaryPath string, timeout time.Duration, logger *zap.Logger) *YtDlp {
	// Circuit breaker configuration
	cbConfig := circuitbreaker.Config{
		MaxRequests: 3,
		Interval:    60 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts circuitbreaker.Counts) bool {
			// Trip if 5+ consecutive failures OR 60%+ failure rate with 10+ requests
			return counts.ConsecutiveFailures >= 5 ||
				(counts.Requests >= 10 && float64(counts.TotalFailures)/float64(counts.Requests) >= 0.6)
		},
		OnStateChange: func(name string, from circuitbreaker.State, to circuitbreaker.State) {
			logger.Warn("Circuit breaker state changed",
				zap.String("name", name),
				zap.String("from", from.String()),
				zap.String("to", to.String()),
			)
		},
	}

	cb := circuitbreaker.NewCircuitBreaker("yt-dlp", cbConfig)

	// Retry configuration with exponential backoff
	retryConfig := retry.Config{
		MaxAttempts:     3,
		InitialDelay:    500 * time.Millisecond,
		MaxDelay:        10 * time.Second,
		Multiplier:      2.0,
		Jitter:          0.3,
		RetryableErrors: isRetryableError,
		OnRetry: func(attempt int, delay time.Duration, err error) {
			logger.Warn("Retrying yt-dlp operation",
				zap.Int("attempt", attempt),
				zap.Duration("delay", delay),
				zap.Error(err),
			)
		},
	}

	return &YtDlp{
		binaryPath:     binaryPath,
		timeout:        timeout,
		logger:         logger,
		circuitBreaker: cb,
		retryConfig:    retryConfig,
	}
}

// isRetryableError determines if error should trigger retry
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Don't retry circuit breaker open errors
	if err == circuitbreaker.ErrCircuitOpen {
		return false
	}

	errStr := strings.ToLower(err.Error())

	// Retryable errors (transient failures)
	retryablePatterns := []string{
		"timeout",
		"connection reset",
		"connection refused",
		"temporary failure",
		"500", // Server error
		"502", // Bad gateway
		"503", // Service unavailable
		"504", // Gateway timeout
		"network",
		"dns",
	}

	for _, pattern := range retryablePatterns {
		if strings.Contains(errStr, pattern) {
			return true
		}
	}

	// Special handling for rate-limit (429) - retry with longer delay
	if strings.Contains(errStr, "429") || strings.Contains(errStr, "rate") {
		return true
	}

	// Non-retryable errors (permanent failures)
	nonRetryablePatterns := []string{
		"404",         // Not found
		"403",         // Forbidden
		"401",         // Unauthorized
		"unsupported", // Unsupported site
		"private",     // Private video
		"copyright",   // Copyright claim
		"unavailable", // Video unavailable
		"removed",     // Video removed
		"invalid",     // Invalid input
		"malformed",   // Malformed request
	}

	for _, pattern := range nonRetryablePatterns {
		if strings.Contains(errStr, pattern) {
			return false
		}
	}

	// Default: retry unknown errors (conservative approach)
	return true
}

// ExtractMetadata extracts metadata from a URL without downloading
// Uses circuit breaker and retry logic for resilience
func (y *YtDlp) ExtractMetadata(ctx context.Context, url string) (*types.MediaMetadata, error) {
	var metadata *types.MediaMetadata

	// Wrap with circuit breaker and retry logic
	err := y.circuitBreaker.Execute(ctx, func() error {
		return retry.Retry(ctx, y.retryConfig, func() error {
			args := []string{
				"--no-playlist", // Single video only
				"--no-warnings",
				"--skip-download", // Metadata only
				"--print-json",    // One JSON object per item
			}

			// Add YouTube-specific headers to bypass bot detection
			if strings.Contains(url, "youtube.com") || strings.Contains(url, "youtu.be") {
				args = append(args,
					"--user-agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
					"--extractor-args", "youtube:player_client=android_vr,web",
				)
			}

			args = append(args, url)

			output, err := y.execute(ctx, args)
			if err != nil {
				return fmt.Errorf("failed to extract metadata: %w", err)
			}

			rawData, err := extractJSONObjectFromOutput(output)
			if err != nil {
				return fmt.Errorf("failed to parse metadata: %w", err)
			}

			metadata = y.parseMetadata(rawData)
			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	return metadata, nil
}

func extractJSONObjectFromOutput(output string) (map[string]interface{}, error) {
	// yt-dlp may print non-JSON lines (e.g., warnings, progress). We scan for the
	// last JSON object-looking line and unmarshal that.
	lines := strings.Split(output, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "{") && strings.HasSuffix(line, "}") {
			var raw map[string]interface{}
			if err := json.Unmarshal([]byte(line), &raw); err == nil {
				return raw, nil
			}
		}
	}
	return nil, fmt.Errorf("no JSON object found in yt-dlp output")
}

// Download downloads media from URL to specified output path
// Uses circuit breaker and retry logic for resilient downloads
func (y *YtDlp) Download(ctx context.Context, url, outputPath string, opts DownloadOptions) (*types.MediaMetadata, error) {
	var metadata *types.MediaMetadata

	// Wrap with circuit breaker and retry logic
	err := y.circuitBreaker.Execute(ctx, func() error {
		return retry.Retry(ctx, y.retryConfig, func() error {
			args := y.buildDownloadArgs(url, outputPath, opts)

			y.logger.Info("Starting download",
				zap.String("url", url),
				zap.String("output", outputPath),
				zap.Strings("args", args),
			)

			// Execute with progress tracking
			var err error
			metadata, err = y.downloadWithProgress(ctx, args, opts.ProgressCallback)
			if err != nil {
				return fmt.Errorf("download failed: %w", err)
			}

			return nil
		})
	})

	if err != nil {
		return nil, err
	}

	return metadata, nil
}

// DownloadOptions configures the download behavior
type DownloadOptions struct {
	Quality          string
	Format           string
	ExtractAudio     bool
	AudioFormat      string
	AudioBitrate     string
	Subtitles        []string
	CookiesFile      string
	UserAgent        string
	ProxyURL         string
	ProgressCallback func(progress int)
}

// buildDownloadArgs constructs yt-dlp command arguments
func (y *YtDlp) buildDownloadArgs(url, outputPath string, opts DownloadOptions) []string {
	args := []string{
		"--no-playlist",
		"--no-warnings",
		"--progress",
		"--newline", // Progress on new lines for parsing
		"-o", outputPath,
	}

	// Ensure yt-dlp can find ffmpeg for merging separate streams (esp. on Windows
	// when ffmpeg.exe isn't on PATH).
	if ffmpegPath := strings.TrimSpace(os.Getenv("FFMPEG_PATH")); ffmpegPath != "" {
		args = append(args, "--ffmpeg-location", ffmpegPath)
	}

	// Quality and format selection
	if opts.ExtractAudio {
		args = append(args,
			"-x", // Extract audio
			"--audio-format", opts.AudioFormat,
		)
		if opts.AudioBitrate != "" {
			args = append(args, "--audio-quality", opts.AudioBitrate)
		}
	} else {
		formatStr := y.buildFormatString(opts.Quality, opts.Format)
		args = append(args, "-f", formatStr)

		// For MP4: Use smart codec handling - copy video, ensure AAC audio
		if opts.Format == "mp4" || opts.Format == "" {
			// **PERFORMANCE & COMPATIBILITY OPTIMIZATION**:
			// - Video: Copy if already h264/avc1 (10x faster, most YouTube videos)
			// - Audio: Always encode to AAC (universal compatibility, fast ~20-30s)
			// Why? Opus audio in MP4 breaks Windows Media Player and many players
			// This gives us 90% of speed benefit with 100% compatibility
			args = append(args,
				"--postprocessor-args",
				"ffmpeg:-c:v copy -c:a aac -b:a 192k -movflags +faststart",
			)
		}

		// For non-standard formats, use remux/recode
		if opts.Format != "" && opts.Format != "mp4" && opts.Format != "webm" {
			args = append(args, "--recode-video", opts.Format)
		} else if opts.Format != "" {
			// Prefer merging into requested container
			args = append(args, "--merge-output-format", opts.Format)
		}
	}

	// Subtitles
	if len(opts.Subtitles) > 0 {
		args = append(args,
			"--write-subs",
			"--write-auto-subs",
			"--sub-langs", strings.Join(opts.Subtitles, ","),
			"--convert-subs", "srt",
		)
	}

	// Authentication and anti-detection
	if opts.CookiesFile != "" {
		args = append(args, "--cookies", opts.CookiesFile)
	}

	// Default User-Agent if not provided
	ua := opts.UserAgent
	if ua == "" {
		ua = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
	}
	args = append(args, "--user-agent", ua)

	if opts.ProxyURL != "" {
		args = append(args, "--proxy", opts.ProxyURL)
	}

	// Platform-specific optimizations
	args = append(args,
		"--extractor-args", "youtube:player_client=android_vr,web",
	)

	args = append(args, url)

	return args
}

// buildFormatString creates yt-dlp format selection string with platform-aware fallbacks
func (y *YtDlp) buildFormatString(quality, format string) string {
	var formatStr string

	// YouTube generally provides these codec pairs that merge properly:
	// - bestvideo[ext=mp4][vcodec^=avc1] + bestaudio[ext=m4a][acodec=aac]
	// - Fallback: bestvideo[height<=X][vcodec^=avc1] + bestaudio[acodec=aac]
	// - Final fallback: bestvideo[height<=X] + bestaudio

	// TikTok: MP4 format may not always be available with strict codec requirements
	// Use more lenient selection for TikTok to avoid "Requested format is not available" errors

	switch quality {
	case "4k":
		if format == "mp4" || format == "" {
			// MP4 with fallback to best available (handles TikTok format availability)
			formatStr = "bestvideo[height<=2160][vcodec^=avc1]+bestaudio[acodec=aac]/bestvideo[height<=2160]+bestaudio/best[height<=2160]/best"
		} else {
			formatStr = "bestvideo[height<=2160]+bestaudio/best[height<=2160]/best"
		}
	case "1080p":
		if format == "mp4" || format == "" {
			formatStr = "bestvideo[height<=1080][vcodec^=avc1]+bestaudio[acodec=aac]/bestvideo[height<=1080]+bestaudio/best[height<=1080]/best"
		} else {
			formatStr = "bestvideo[height<=1080]+bestaudio/best[height<=1080]/best"
		}
	case "720p":
		if format == "mp4" || format == "" {
			formatStr = "bestvideo[height<=720][vcodec^=avc1]+bestaudio[acodec=aac]/bestvideo[height<=720]+bestaudio/best[height<=720]/best"
		} else {
			formatStr = "bestvideo[height<=720]+bestaudio/best[height<=720]/best"
		}
	case "480p":
		if format == "mp4" || format == "" {
			formatStr = "bestvideo[height<=480][vcodec^=avc1]+bestaudio[acodec=aac]/bestvideo[height<=480]+bestaudio/best[height<=480]/best"
		} else {
			formatStr = "bestvideo[height<=480]+bestaudio/best[height<=480]/best"
		}
	default:
		if format == "mp4" || format == "" {
			// Lenient MP4 selection: try strict h264+aac first, then fallback to any format
			formatStr = "bestvideo[vcodec^=avc1]+bestaudio[acodec=aac]/bestvideo+bestaudio[acodec=aac]/bestvideo+bestaudio/best"
		} else {
			formatStr = "bestvideo+bestaudio/best"
		}
	}

	return formatStr
}

// downloadWithProgress executes download with real-time progress tracking
func (y *YtDlp) downloadWithProgress(ctx context.Context, args []string, callback func(int)) (*types.MediaMetadata, error) {
	ctx, cancel := context.WithTimeout(ctx, y.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, y.binaryPath, args...)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start yt-dlp: %w", err)
	}

	progressRegex := regexp.MustCompile(`\[download\]\s+(\d+(?:\.\d+)?)%`)
	var metadataJSON string
	var stderrBuf bytes.Buffer
	var mu sync.Mutex

	scan := func(r *bufio.Scanner, onLine func(string)) {
		for r.Scan() {
			onLine(r.Text())
		}
	}

	parseLine := func(line string, captureJSON bool) {
		// Progress lines typically go to stderr, but we parse both streams.
		if matches := progressRegex.FindStringSubmatch(line); len(matches) > 1 {
			if progress, err := strconv.ParseFloat(matches[1], 64); err == nil && callback != nil {
				callback(int(progress))
			}
		}
		if captureJSON {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") {
				mu.Lock()
				metadataJSON = trimmed
				mu.Unlock()
			}
		}
	}

	// stdout: capture JSON + progress
	go func() {
		scanner := bufio.NewScanner(stdout)
		// yt-dlp --print-json can emit very large single-line JSON.
		scanner.Buffer(make([]byte, 64*1024), 10*1024*1024)
		scan(scanner, func(line string) {
			parseLine(line, true)
			y.logger.Debug("yt-dlp stdout", zap.String("line", line))
		})
	}()

	// stderr: capture progress + errors
	go func() {
		scanner := bufio.NewScanner(stderr)
		scanner.Buffer(make([]byte, 64*1024), 512*1024)
		scan(scanner, func(line string) {
			parseLine(line, false)
			mu.Lock()
			stderrBuf.WriteString(line + "\n")
			mu.Unlock()
			y.logger.Debug("yt-dlp stderr", zap.String("line", line))
		})
	}()

	if err := cmd.Wait(); err != nil {
		mu.Lock()
		errMsg := stderrBuf.String()
		mu.Unlock()
		y.logger.Error("yt-dlp failed",
			zap.Error(err),
			zap.String("stderr", errMsg),
		)
		return nil, fmt.Errorf("yt-dlp error: %w - %s", err, errMsg)
	}

	// Parse metadata from JSON output
	mu.Lock()
	metadataJSONCopy := metadataJSON
	mu.Unlock()
	if metadataJSONCopy != "" {
		var rawData map[string]interface{}
		if err := json.Unmarshal([]byte(metadataJSONCopy), &rawData); err == nil {
			return y.parseMetadata(rawData), nil
		}
	}

	return nil, nil
}

// execute runs yt-dlp and returns stdout
func (y *YtDlp) execute(ctx context.Context, args []string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, y.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, y.binaryPath, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("command failed: %w - %s", err, string(output))
	}

	return string(output), nil
}

// parseMetadata converts yt-dlp JSON to MediaMetadata struct
func (y *YtDlp) parseMetadata(data map[string]interface{}) *types.MediaMetadata {
	metadata := &types.MediaMetadata{}

	if title, ok := data["title"].(string); ok {
		metadata.Title = title
	}

	if description, ok := data["description"].(string); ok {
		metadata.Description = description
	}

	if duration, ok := data["duration"].(float64); ok {
		metadata.Duration = int(duration)
	}

	if uploader, ok := data["uploader"].(string); ok {
		metadata.Uploader = uploader
	}

	if uploadDate, ok := data["upload_date"].(string); ok {
		metadata.UploadDate = uploadDate
	}

	if viewCount, ok := data["view_count"].(float64); ok {
		metadata.ViewCount = int64(viewCount)
	}

	if likeCount, ok := data["like_count"].(float64); ok {
		metadata.LikeCount = int64(likeCount)
	}

	if thumbnail, ok := data["thumbnail"].(string); ok {
		metadata.Thumbnail = thumbnail
	}

	if extractor, ok := data["extractor"].(string); ok {
		metadata.Platform = strings.ToLower(extractor)
	}

	if width, ok := data["width"].(float64); ok {
		metadata.Width = int(width)
	}

	if height, ok := data["height"].(float64); ok {
		metadata.Height = int(height)
	}

	if fps, ok := data["fps"].(float64); ok {
		metadata.FPS = fps
	}

	if vcodec, ok := data["vcodec"].(string); ok {
		metadata.VideoCodec = vcodec
	}

	if acodec, ok := data["acodec"].(string); ok {
		metadata.AudioCodec = acodec
	}

	// Parse formats array from yt-dlp metadata
	if formatsRaw, ok := data["formats"].([]interface{}); ok {
		metadata.Formats = make([]types.FormatEntry, 0, len(formatsRaw))
		for _, formatRaw := range formatsRaw {
			if formatMap, ok := formatRaw.(map[string]interface{}); ok {
				format := types.FormatEntry{}

				if fid, ok := formatMap["format_id"].(string); ok {
					format.FormatID = fid
				}
				if ext, ok := formatMap["ext"].(string); ok {
					format.Ext = ext
				}
				if quality, ok := formatMap["quality"].(string); ok {
					format.Quality = quality
				}
				if resolution, ok := formatMap["resolution"].(string); ok {
					format.Resolution = resolution
				}
				if width, ok := formatMap["width"].(float64); ok {
					format.Width = int(width)
				}
				if height, ok := formatMap["height"].(float64); ok {
					format.Height = int(height)
				}
				if filesize, ok := formatMap["filesize"].(float64); ok {
					format.Filesize = int64(filesize)
				}
				if bitrate, ok := formatMap["tbr"].(float64); ok {
					format.Bitrate = int(bitrate)
				}
				if vcodec, ok := formatMap["vcodec"].(string); ok {
					format.VideoCodec = vcodec
				}
				if acodec, ok := formatMap["acodec"].(string); ok {
					format.AudioCodec = acodec
				}

				// Build quality label from height if not present
				if format.Quality == "" && format.Height > 0 {
					format.Quality = fmt.Sprintf("%dp", format.Height)
				}

				metadata.Formats = append(metadata.Formats, format)
			}
		}
	}

	return metadata
}

// WriteCookiesFile writes cookies to a Netscape format file
func WriteCookiesFile(cookiesBase64 string) (string, error) {
	tmpFile := filepath.Join(os.TempDir(), fmt.Sprintf("cookies-%d.txt", time.Now().Unix()))

	// Decode base64 if needed
	content := cookiesBase64

	if err := os.WriteFile(tmpFile, []byte(content), 0600); err != nil {
		return "", fmt.Errorf("failed to write cookies file: %w", err)
	}

	return tmpFile, nil
}
