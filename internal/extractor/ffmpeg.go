package extractor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	ffmpeg "github.com/u2takey/ffmpeg-go"
	"go.uber.org/zap"
)

// FFmpeg wraps ffmpeg-go for media transcoding
type FFmpeg struct {
	binaryPath string
	timeout    time.Duration
	logger     *zap.Logger
}

// NewFFmpeg creates a new FFmpeg wrapper
func NewFFmpeg(binaryPath string, timeout time.Duration, logger *zap.Logger) *FFmpeg {
	return &FFmpeg{
		binaryPath: binaryPath,
		timeout:    timeout,
		logger:     logger,
	}
}

// ExtractAudio extracts audio from video file
func (f *FFmpeg) ExtractAudio(ctx context.Context, inputPath, format, bitrate string) (string, error) {
	outputPath := f.generateOutputPath(inputPath, format)

	f.logger.Info("Extracting audio",
		zap.String("input", inputPath),
		zap.String("output", outputPath),
		zap.String("format", format),
		zap.String("bitrate", bitrate),
	)

	kwargs := ffmpeg.KwArgs{
		"vn":     "", // No video
		"acodec": f.getAudioCodec(format),
	}

	if bitrate != "" {
		kwargs["audio_bitrate"] = bitrate
	}

	err := ffmpeg.Input(inputPath).
		Output(outputPath, kwargs).
		OverWriteOutput().
		ErrorToStdOut().
		SetFfmpegPath(f.binaryPath).
		Run()

	if err != nil {
		return "", fmt.Errorf("audio extraction failed: %w", err)
	}

	return outputPath, nil
}

// ConvertFormat converts video between formats
func (f *FFmpeg) ConvertFormat(ctx context.Context, inputPath, outputFormat, codec, bitrate string) (string, error) {
	if strings.TrimSpace(outputFormat) == "" {
		return "", fmt.Errorf("output format is required")
	}

	ext := outputFormat
	if ext[0] != '.' {
		ext = "." + ext
	}

	outputPath := f.changeExtension(inputPath, ext)

	// If the target container extension is the same as the input (e.g. mp4->mp4)
	// we must still write to a different file. FFmpeg cannot edit files in-place.
	inExt := strings.ToLower(filepath.Ext(inputPath))
	outExt := strings.ToLower(ext)
	if inExt == outExt {
		outputPath = f.appendSuffix(inputPath, "_converted")
	}

	inClean := filepath.Clean(inputPath)
	outClean := filepath.Clean(outputPath)
	if strings.EqualFold(inClean, outClean) {
		outputPath = f.appendSuffix(inputPath, "_converted")
	}

	f.logger.Info("Converting format",
		zap.String("input", inputPath),
		zap.String("output", outputPath),
		zap.String("codec", codec),
	)

	kwargs := ffmpeg.KwArgs{
		"c:v": codec,
	}

	if bitrate != "" {
		kwargs["b:v"] = bitrate
	}

	// Copy audio stream if possible
	kwargs["c:a"] = "copy"

	err := ffmpeg.Input(inputPath).
		Output(outputPath, kwargs).
		OverWriteOutput().
		ErrorToStdOut().
		SetFfmpegPath(f.binaryPath).
		Run()

	if err != nil {
		return "", fmt.Errorf("format conversion failed: %w", err)
	}

	return outputPath, nil
}

// DownscaleVideo reduces video resolution
func (f *FFmpeg) DownscaleVideo(ctx context.Context, inputPath string, maxHeight int, codec, bitrate string) (string, error) {
	outputPath := f.appendSuffix(inputPath, fmt.Sprintf("_%dp", maxHeight))

	f.logger.Info("Downscaling video",
		zap.String("input", inputPath),
		zap.String("output", outputPath),
		zap.Int("max_height", maxHeight),
	)

	// Scale filter: scale=-2:HEIGHT (maintains aspect ratio)
	scaleFilter := fmt.Sprintf("scale=-2:%d", maxHeight)

	kwargs := ffmpeg.KwArgs{
		"c:v": codec,
		"c:a": "copy",
		"vf":  scaleFilter, // Use vf parameter instead of Filter()
	}

	if bitrate != "" {
		kwargs["b:v"] = bitrate
	}

	err := ffmpeg.Input(inputPath).
		Output(outputPath, kwargs).
		OverWriteOutput().
		ErrorToStdOut().
		SetFfmpegPath(f.binaryPath).
		Run()

	if err != nil {
		return "", fmt.Errorf("downscaling failed: %w", err)
	}

	f.logger.Info("Video downscaled",
		zap.String("scale_filter", scaleFilter),
	)

	return outputPath, nil
}

// ExtractSubtitles extracts subtitle streams from video
func (f *FFmpeg) ExtractSubtitles(ctx context.Context, inputPath string, language string) ([]string, error) {
	outputPath := f.changeExtension(inputPath, "."+language+".srt")

	f.logger.Info("Extracting subtitles",
		zap.String("input", inputPath),
		zap.String("language", language),
	)

	err := ffmpeg.Input(inputPath).
		Output(outputPath, ffmpeg.KwArgs{
			"c:s": "srt",
			"map": "0:s:0", // First subtitle stream
		}).
		OverWriteOutput().
		ErrorToStdOut().
		SetFfmpegPath(f.binaryPath).
		Run()

	if err != nil {
		return nil, fmt.Errorf("subtitle extraction failed: %w", err)
	}

	return []string{outputPath}, nil
}

// GetMediaInfo retrieves information about a media file
func (f *FFmpeg) GetMediaInfo(inputPath string) (map[string]interface{}, error) {
	// Use ffprobe to get media information
	// This is a simplified version - in production, use ffprobe directly
	info := make(map[string]interface{})

	stat, err := os.Stat(inputPath)
	if err != nil {
		return nil, err
	}

	info["size"] = stat.Size()
	info["path"] = inputPath

	return info, nil
}

// CompressVideo compresses video with quality vs speed preset
func (f *FFmpeg) CompressVideo(ctx context.Context, inputPath, preset string) (string, error) {
	outputPath := f.appendSuffix(inputPath, "_compressed")

	f.logger.Info("Compressing video",
		zap.String("input", inputPath),
		zap.String("preset", preset),
	)

	err := ffmpeg.Input(inputPath).
		Output(outputPath, ffmpeg.KwArgs{
			"c:v":    "libx264",
			"preset": preset, // ultrafast, fast, medium, slow
			"crf":    "23",   // Quality (0-51, lower = better)
			"c:a":    "aac",
			"b:a":    "128k",
		}).
		OverWriteOutput().
		ErrorToStdOut().
		SetFfmpegPath(f.binaryPath).
		Run()

	if err != nil {
		return "", fmt.Errorf("compression failed: %w", err)
	}

	return outputPath, nil
}

// getAudioCodec maps audio format to FFmpeg codec
func (f *FFmpeg) getAudioCodec(format string) string {
	codecMap := map[string]string{
		"mp3":  "libmp3lame",
		"aac":  "aac",
		"flac": "flac",
		"opus": "libopus",
		"wav":  "pcm_s16le",
	}

	if codec, ok := codecMap[format]; ok {
		return codec
	}

	return "copy" // Default to stream copy
}

// generateOutputPath creates output path with new extension
func (f *FFmpeg) generateOutputPath(inputPath, format string) string {
	ext := format
	if ext[0] != '.' {
		ext = "." + ext
	}

	return f.changeExtension(inputPath, ext)
}

// changeExtension replaces file extension
func (f *FFmpeg) changeExtension(path, newExt string) string {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	name := base[:len(base)-len(filepath.Ext(base))]

	return filepath.Join(dir, name+newExt)
}

// appendSuffix adds suffix before extension
func (f *FFmpeg) appendSuffix(path, suffix string) string {
	ext := filepath.Ext(path)
	base := path[:len(path)-len(ext)]

	return base + suffix + ext
}
