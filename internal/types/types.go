package types

import "time"

// ExtractionRequest represents a media extraction job request
type ExtractionRequest struct {
	URL           string   `json:"url" validate:"required,url"`
	Quality       string   `json:"quality"`                  // 4k, 1080p, 720p, 480p, best
	Format        string   `json:"format"`                   // mp4, avi, mkv, webm
	ExtractAudio  bool     `json:"extract_audio"`            // Extract audio only
	AudioFormat   string   `json:"audio_format"`             // mp3, aac, flac
	AudioBitrate  string   `json:"audio_bitrate"`            // 128k, 192k, 320k
	Subtitles     []string `json:"subtitles"`                // ["en", "tr"]
	CookiesBase64 string   `json:"cookies_base64,omitempty"` // Base64 encoded cookie file
	UserAgent     string   `json:"user_agent,omitempty"`     // Custom user agent
	ProxyURL      string   `json:"proxy_url,omitempty"`      // Custom proxy
	WebhookURL    string   `json:"webhook_url,omitempty"`    // Callback URL on completion
}

// ExtractionJob represents a job in the queue
type ExtractionJob struct {
	ID        string            `json:"id"`
	Request   ExtractionRequest `json:"request"`
	Status    JobStatus         `json:"status"`
	Progress  int               `json:"progress"` // 0-100
	Error     string            `json:"error,omitempty"`
	Metadata  *MediaMetadata    `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
	Result    *ExtractionResult `json:"result,omitempty"`
}

// JobStatus represents the current state of a job
type JobStatus string

const (
	StatusPending    JobStatus = "pending"
	StatusProcessing JobStatus = "processing"
	StatusCompleted  JobStatus = "completed"
	StatusFailed     JobStatus = "failed"
)

// MediaMetadata contains information about the extracted media
type MediaMetadata struct {
	Title       string        `json:"title"`
	Description string        `json:"description,omitempty"`
	Duration    int           `json:"duration"` // seconds
	Uploader    string        `json:"uploader,omitempty"`
	UploadDate  string        `json:"upload_date,omitempty"`
	ViewCount   int64         `json:"view_count,omitempty"`
	LikeCount   int64         `json:"like_count,omitempty"`
	Thumbnail   string        `json:"thumbnail,omitempty"`
	Platform    string        `json:"platform"` // youtube, instagram, tiktok, twitter
	Width       int           `json:"width,omitempty"`
	Height      int           `json:"height,omitempty"`
	FPS         float64       `json:"fps,omitempty"`
	VideoCodec  string        `json:"video_codec,omitempty"`
	AudioCodec  string        `json:"audio_codec,omitempty"`
	Formats     []FormatEntry `json:"formats,omitempty"` // Available formats from yt-dlp
}

// FormatEntry represents a single format option from yt-dlp
type FormatEntry struct {
	FormatID   string `json:"format_id"`
	Ext        string `json:"ext"`
	Quality    string `json:"quality,omitempty"`
	Resolution string `json:"resolution,omitempty"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	Filesize   int64  `json:"filesize,omitempty"`
	Bitrate    int    `json:"bitrate,omitempty"`
	Codec      string `json:"codec,omitempty"`
	VideoCodec string `json:"vcodec,omitempty"`
	AudioCodec string `json:"acodec,omitempty"`
}

// ExtractionResult contains the output of a successful extraction
type ExtractionResult struct {
	DownloadURL  string    `json:"download_url"` // Presigned S3 URL
	Filename     string    `json:"filename"`
	SizeBytes    int64     `json:"size_bytes"`
	Format       string    `json:"format"`
	SubtitleURLs []string  `json:"subtitle_urls,omitempty"` // Multiple language subtitles
	ThumbnailURL string    `json:"thumbnail_url,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"` // Presigned URL expiry
}

// Platform represents supported platforms
type Platform string

const (
	PlatformYouTube   Platform = "youtube"
	PlatformInstagram Platform = "instagram"
	PlatformTikTok    Platform = "tiktok"
	PlatformTwitter   Platform = "twitter"
	PlatformUnknown   Platform = "unknown"
)

// QualityPreset defines video quality settings
type QualityPreset struct {
	Name         string
	MaxHeight    int
	VideoBitrate string
	AudioBitrate string
}

var QualityPresets = map[string]QualityPreset{
	"4k":    {Name: "4k", MaxHeight: 2160, VideoBitrate: "12M", AudioBitrate: "256k"},
	"1080p": {Name: "1080p", MaxHeight: 1080, VideoBitrate: "5M", AudioBitrate: "192k"},
	"720p":  {Name: "720p", MaxHeight: 720, VideoBitrate: "2.5M", AudioBitrate: "128k"},
	"480p":  {Name: "480p", MaxHeight: 480, VideoBitrate: "1M", AudioBitrate: "128k"},
	"best":  {Name: "best", MaxHeight: 0, VideoBitrate: "", AudioBitrate: ""},
}

// HistoryItem represents a download entry in site history
type HistoryItem struct {
	URL       string `json:"url"`
	Title     string `json:"title"`
	Thumbnail string `json:"thumbnail"`
	Format    string `json:"format"`
	Quality   string `json:"quality"`
	Duration  int    `json:"duration"`
	Platform  string `json:"platform"`
	Timestamp int64  `json:"timestamp"`
}
