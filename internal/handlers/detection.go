package handlers

import (
	"context"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"github.com/KeremKalyoncu/MedYan/internal/extractor"
	"github.com/KeremKalyoncu/MedYan/internal/types"
)

// DetectionHandler handles smart URL detection and analysis
type DetectionHandler struct {
	ytdlp  *extractor.YtDlp
	logger *zap.Logger
}

// NewDetectionHandler creates a new detection handler
func NewDetectionHandler(ytdlp *extractor.YtDlp, logger *zap.Logger) *DetectionHandler {
	return &DetectionHandler{
		ytdlp:  ytdlp,
		logger: logger,
	}
}

// PlatformInfo holds platform-specific information
type PlatformInfo struct {
	Platform           string   `json:"platform"`
	PlatformName       string   `json:"platform_name"`
	SupportedFormats   []string `json:"supported_formats"`
	SupportedQualities []string `json:"supported_qualities"`
	SupportsAudio      bool     `json:"supports_audio"`
	SupportsVideo      bool     `json:"supports_video"`
	RequiresAuth       bool     `json:"requires_auth,omitempty"`
}

// VideoInfo holds detailed video information
type VideoInfo struct {
	URL               string       `json:"url"`
	Title             string       `json:"title"`
	Description       string       `json:"description,omitempty"`
	Platform          PlatformInfo `json:"platform"`
	Duration          int          `json:"duration"`
	Thumbnail         string       `json:"thumbnail,omitempty"`
	AvailableFormats  []FormatInfo `json:"available_formats"`
	RecommendedFormat *FormatInfo  `json:"recommended_format"`
}

// FormatInfo holds format details
type FormatInfo struct {
	FormatID   string `json:"format_id"`
	Ext        string `json:"ext"`
	Quality    string `json:"quality"`
	Resolution string `json:"resolution,omitempty"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	FileSize   int64  `json:"filesize,omitempty"`
	Bitrate    int    `json:"bitrate,omitempty"`
	Codec      string `json:"codec,omitempty"`
	HasAudio   bool   `json:"has_audio"`
	HasVideo   bool   `json:"has_video"`
}

// DetectURL analyzes URL and returns platform info + available options
func (h *DetectionHandler) DetectURL(c *fiber.Ctx) error {
	type Request struct {
		URL string `json:"url"`
	}

	var req Request
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	if req.URL == "" {
		return c.Status(400).JSON(fiber.Map{
			"error": "URL is required",
		})
	}

	// Detect platform
	platform := detectPlatformFromURL(req.URL)
	platformInfo := getPlatformInfo(platform)

	// Try to extract metadata to get detailed info
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	metadata, err := h.ytdlp.ExtractMetadata(ctx, req.URL)
	if err != nil {
		h.logger.Warn("Failed to extract full metadata, returning basic platform info",
			zap.Error(err),
			zap.String("url", req.URL),
		)

		// Return basic platform info without metadata
		return c.JSON(fiber.Map{
			"url":      req.URL,
			"platform": platformInfo,
			"error":    err.Error(),
			"message":  "Could not fetch video details, but platform detected",
		})
	}

	// Build video info with metadata
	videoInfo := h.buildVideoInfo(req.URL, metadata, platformInfo)

	return c.JSON(videoInfo)
}

// buildVideoInfo constructs detailed video information
func (h *DetectionHandler) buildVideoInfo(url string, metadata *types.MediaMetadata, platformInfo PlatformInfo) *VideoInfo {
	info := &VideoInfo{
		URL:         url,
		Title:       metadata.Title,
		Description: metadata.Description,
		Platform:    platformInfo,
		Duration:    metadata.Duration,
		Thumbnail:   metadata.Thumbnail,
	}

	// Extract available formats from metadata
	formats := h.parseAvailableFormats(metadata, platformInfo)
	info.AvailableFormats = formats

	// Recommend best format
	if len(formats) > 0 {
		info.RecommendedFormat = h.recommendBestFormat(formats)
	}

	return info
}

// parseAvailableFormats extracts format information
func (h *DetectionHandler) parseAvailableFormats(metadata *types.MediaMetadata, platform PlatformInfo) []FormatInfo {
	formats := []FormatInfo{}

	// Parse from metadata formats if available
	if len(metadata.Formats) > 0 {
		seen := make(map[string]bool)
		for _, f := range metadata.Formats {
			// Create unique key
			key := f.Quality + "_" + f.Ext

			if seen[key] {
				continue
			}
			seen[key] = true

			formats = append(formats, FormatInfo{
				FormatID:   f.FormatID,
				Ext:        f.Ext,
				Quality:    f.Quality,
				Resolution: f.Resolution,
				Width:      f.Width,
				Height:     f.Height,
				FileSize:   f.Filesize,
				Bitrate:    f.Bitrate,
				Codec:      f.Codec,
				HasAudio:   f.AudioCodec != "",
				HasVideo:   f.VideoCodec != "",
			})
		}
	}

	// If no formats, generate standard options based on platform
	if len(formats) == 0 {
		formats = h.generateStandardFormats(platform)
	}

	return formats
}

// generateStandardFormats creates default format options
func (h *DetectionHandler) generateStandardFormats(platform PlatformInfo) []FormatInfo {
	formats := []FormatInfo{}

	// Video formats
	if platform.SupportsVideo {
		for _, quality := range platform.SupportedQualities {
			for _, ext := range platform.SupportedFormats {
				if ext == "mp3" || ext == "m4a" {
					continue // Skip audio-only formats
				}

				formats = append(formats, FormatInfo{
					Ext:      ext,
					Quality:  quality,
					HasAudio: true,
					HasVideo: true,
				})
			}
		}
	}

	// Audio formats
	if platform.SupportsAudio {
		audioFormats := []string{"mp3", "m4a", "opus"}
		for _, ext := range audioFormats {
			formats = append(formats, FormatInfo{
				Ext:      ext,
				Quality:  "audio",
				HasAudio: true,
				HasVideo: false,
			})
		}
	}

	return formats
}

// recommendBestFormat selects the best format
func (h *DetectionHandler) recommendBestFormat(formats []FormatInfo) *FormatInfo {
	var best *FormatInfo

	for i := range formats {
		format := &formats[i]

		// Prefer video+audio combined
		if !format.HasAudio || !format.HasVideo {
			continue
		}

		// Prefer mp4 (universal compatibility)
		if format.Ext != "mp4" {
			continue
		}

		// Prefer 1080p or highest available
		if format.Height > 0 {
			if best == nil || format.Height > best.Height {
				best = format
			}
		}
	}

	// Fallback: first format with video+audio
	if best == nil {
		for i := range formats {
			format := &formats[i]
			if format.HasAudio && format.HasVideo {
				best = format
				break
			}
		}
	}

	// Ultimate fallback: first format
	if best == nil && len(formats) > 0 {
		best = &formats[0]
	}

	return best
}

// detectPlatformFromURL identifies platform from URL
func detectPlatformFromURL(url string) string {
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
	if strings.Contains(url, "dailymotion.com") {
		return "dailymotion"
	}
	if strings.Contains(url, "twitch.tv") {
		return "twitch"
	}
	if strings.Contains(url, "reddit.com") {
		return "reddit"
	}

	return "other"
}

// getPlatformInfo returns platform-specific capabilities
func getPlatformInfo(platform string) PlatformInfo {
	switch platform {
	case "youtube":
		return PlatformInfo{
			Platform:           "youtube",
			PlatformName:       "YouTube",
			SupportedFormats:   []string{"mp4", "webm", "mkv"},
			SupportedQualities: []string{"360p", "480p", "720p", "1080p", "1440p", "2160p"},
			SupportsAudio:      true,
			SupportsVideo:      true,
			RequiresAuth:       false,
		}

	case "instagram":
		return PlatformInfo{
			Platform:           "instagram",
			PlatformName:       "Instagram",
			SupportedFormats:   []string{"mp4"},
			SupportedQualities: []string{"720p", "1080p"},
			SupportsAudio:      true,
			SupportsVideo:      true,
			RequiresAuth:       true, // Often requires cookies
		}

	case "tiktok":
		return PlatformInfo{
			Platform:           "tiktok",
			PlatformName:       "TikTok",
			SupportedFormats:   []string{"mp4"},
			SupportedQualities: []string{"720p", "1080p"},
			SupportsAudio:      true,
			SupportsVideo:      true,
			RequiresAuth:       false,
		}

	case "twitter":
		return PlatformInfo{
			Platform:           "twitter",
			PlatformName:       "Twitter/X",
			SupportedFormats:   []string{"mp4"},
			SupportedQualities: []string{"360p", "720p"},
			SupportsAudio:      true,
			SupportsVideo:      true,
			RequiresAuth:       false,
		}

	case "facebook":
		return PlatformInfo{
			Platform:           "facebook",
			PlatformName:       "Facebook",
			SupportedFormats:   []string{"mp4"},
			SupportedQualities: []string{"360p", "720p", "1080p"},
			SupportsAudio:      true,
			SupportsVideo:      true,
			RequiresAuth:       true,
		}

	case "vimeo":
		return PlatformInfo{
			Platform:           "vimeo",
			PlatformName:       "Vimeo",
			SupportedFormats:   []string{"mp4"},
			SupportedQualities: []string{"360p", "480p", "720p", "1080p"},
			SupportsAudio:      true,
			SupportsVideo:      true,
			RequiresAuth:       false,
		}

	case "twitch":
		return PlatformInfo{
			Platform:           "twitch",
			PlatformName:       "Twitch",
			SupportedFormats:   []string{"mp4"},
			SupportedQualities: []string{"360p", "480p", "720p", "1080p"},
			SupportsAudio:      true,
			SupportsVideo:      true,
			RequiresAuth:       false,
		}

	default:
		return PlatformInfo{
			Platform:           "other",
			PlatformName:       "Other",
			SupportedFormats:   []string{"mp4"},
			SupportedQualities: []string{"720p", "1080p"},
			SupportsAudio:      true,
			SupportsVideo:      true,
			RequiresAuth:       false,
		}
	}
}
