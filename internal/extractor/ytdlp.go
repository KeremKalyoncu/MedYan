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

	"github.com/gsker/media-extraction-saas/internal/types"
)

// YtDlp wraps yt-dlp for media extraction
type YtDlp struct {
	binaryPath string
	timeout    time.Duration
	logger     *zap.Logger
}

// NewYtDlp creates a new yt-dlp wrapper
func NewYtDlp(binaryPath string, timeout time.Duration, logger *zap.Logger) *YtDlp {
	return &YtDlp{
		binaryPath: binaryPath,
		timeout:    timeout,
		logger:     logger,
	}
}

// ExtractMetadata extracts metadata from a URL without downloading
func (y *YtDlp) ExtractMetadata(ctx context.Context, url string) (*types.MediaMetadata, error) {
	args := []string{
		"--no-playlist", // Single video only
		"--no-warnings",
		"--skip-download", // Metadata only
		"--print-json",    // One JSON object per item
		url,
	}

	output, err := y.execute(ctx, args)
	if err != nil {
		return nil, fmt.Errorf("failed to extract metadata: %w", err)
	}

	rawData, err := extractJSONObjectFromOutput(output)
	if err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	metadata := y.parseMetadata(rawData)
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
func (y *YtDlp) Download(ctx context.Context, url, outputPath string, opts DownloadOptions) (*types.MediaMetadata, error) {
	args := y.buildDownloadArgs(url, outputPath, opts)

	y.logger.Info("Starting download",
		zap.String("url", url),
		zap.String("output", outputPath),
		zap.Strings("args", args),
	)

	// Execute with progress tracking
	metadata, err := y.downloadWithProgress(ctx, args, opts.ProgressCallback)
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
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

	if opts.UserAgent != "" {
		args = append(args, "--user-agent", opts.UserAgent)
	}

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

// buildFormatString creates yt-dlp format selection string
func (y *YtDlp) buildFormatString(quality, format string) string {
	var formatStr string

	switch quality {
	case "4k":
		formatStr = "bestvideo[height<=2160][ext=mp4]+bestaudio[ext=m4a]/best[height<=2160]"
	case "1080p":
		formatStr = "bestvideo[height<=1080][ext=mp4]+bestaudio[ext=m4a]/best[height<=1080]"
	case "720p":
		formatStr = "bestvideo[height<=720][ext=mp4]+bestaudio[ext=m4a]/best[height<=720]"
	case "480p":
		formatStr = "bestvideo[height<=480][ext=mp4]+bestaudio[ext=m4a]/best[height<=480]"
	default:
		formatStr = "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best"
	}

	// Override extension if specific format requested
	if format != "" && format != "mp4" {
		formatStr = strings.ReplaceAll(formatStr, "[ext=mp4]", fmt.Sprintf("[ext=%s]", format))
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
