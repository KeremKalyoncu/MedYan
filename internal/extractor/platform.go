package extractor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/KeremKalyoncu/MedYan/internal/types"
)

// PlatformExtractor provides platform-specific extraction strategies
type PlatformExtractor struct {
	ytdlp  *YtDlp
	logger *zap.Logger
}

// NewPlatformExtractor creates a new platform-specific extractor
func NewPlatformExtractor(ytdlp *YtDlp, logger *zap.Logger) *PlatformExtractor {
	return &PlatformExtractor{
		ytdlp:  ytdlp,
		logger: logger,
	}
}

// ExtractWithFallback attempts extraction with platform-specific fallbacks
func (p *PlatformExtractor) ExtractWithFallback(ctx context.Context, url string) (*types.MediaMetadata, error) {
	platform := detectPlatform(url)

	switch platform {
	case "instagram":
		return p.extractInstagram(ctx, url)
	case "tiktok":
		return p.extractTikTok(ctx, url)
	case "youtube":
		return p.extractYouTube(ctx, url)
	default:
		return p.ytdlp.ExtractMetadata(ctx, url)
	}
}

// extractInstagram handles Instagram with fallback strategies
func (p *PlatformExtractor) extractInstagram(ctx context.Context, url string) (*types.MediaMetadata, error) {
	p.logger.Info("Extracting Instagram content with fallback strategies", zap.String("url", url))

	// Strategy 1: Try normal extraction first
	metadata, err := p.ytdlp.ExtractMetadata(ctx, url)
	if err == nil {
		p.logger.Info("Instagram extraction successful (standard method)")
		return metadata, nil
	}

	errStr := strings.ToLower(err.Error())

	// Check if error is rate-limit related
	if strings.Contains(errStr, "rate") || strings.Contains(errStr, "login required") {
		p.logger.Warn("Instagram rate-limit or authentication required",
			zap.Error(err),
			zap.String("fallback", "user should retry later or provide cookies"),
		)

		// Return helpful error message
		return nil, fmt.Errorf("Instagram rate-limit reached or login required. Please: 1) Wait 5-10 minutes before retrying, 2) Try a different Instagram URL, or 3) Contact support for authentication options. Original error: %w", err)
	}

	// For other errors, return as-is
	return nil, err
}

// extractTikTok handles TikTok with fallback strategies
func (p *PlatformExtractor) extractTikTok(ctx context.Context, url string) (*types.MediaMetadata, error) {
	p.logger.Info("Extracting TikTok content", zap.String("url", url))

	// TikTok extraction is usually straightforward
	metadata, err := p.ytdlp.ExtractMetadata(ctx, url)
	if err != nil {
		p.logger.Warn("TikTok extraction failed",
			zap.Error(err),
			zap.String("url", url),
		)
		return nil, fmt.Errorf("TikTok extraction failed. The video may be private, deleted, or region-restricted. Original error: %w", err)
	}

	return metadata, nil
}

// extractYouTube handles YouTube with fallback strategies
func (p *PlatformExtractor) extractYouTube(ctx context.Context, url string) (*types.MediaMetadata, error) {
	p.logger.Info("Extracting YouTube content", zap.String("url", url))

	// YouTube usually works well
	metadata, err := p.ytdlp.ExtractMetadata(ctx, url)
	if err != nil {
		errStr := strings.ToLower(err.Error())

		if strings.Contains(errStr, "video unavailable") {
			return nil, fmt.Errorf("YouTube video is unavailable. It may be private, deleted, or region-restricted")
		}

		if strings.Contains(errStr, "copyright") {
			return nil, fmt.Errorf("YouTube video has copyright restrictions and cannot be downloaded")
		}

		return nil, err
	}

	return metadata, nil
}

// detectPlatform identifies platform from URL
func detectPlatform(url string) string {
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

	return "other"
}

// DownloadWithFallback downloads with platform-specific strategies
func (p *PlatformExtractor) DownloadWithFallback(ctx context.Context, url, outputPath string, opts DownloadOptions) (*types.MediaMetadata, error) {
	platform := detectPlatform(url)

	// Add platform-specific tweaks
	switch platform {
	case "instagram":
		// Instagram: Add longer timeout for rate-limit scenarios
		ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
		defer cancel()

		p.logger.Info("Instagram download with extended timeout")
		return p.ytdlp.Download(ctx, url, outputPath, opts)

	case "tiktok":
		// TikTok: Sometimes needs multiple attempts
		p.logger.Info("TikTok download")
		return p.ytdlp.Download(ctx, url, outputPath, opts)

	default:
		return p.ytdlp.Download(ctx, url, outputPath, opts)
	}
}
