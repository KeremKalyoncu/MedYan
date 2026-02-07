package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all application configuration
type Config struct {
	// API Configuration
	API APIConfig

	// Redis Configuration
	Redis RedisConfig

	// S3/Storage Configuration
	Storage StorageConfig

	// Extractor Configuration
	Extractor ExtractorConfig

	// Worker Configuration
	Worker WorkerConfig

	// Logging Configuration
	Logger LoggerConfig

	// Cache Configuration
	Cache CacheConfig

	// Cleanup Configuration
	Cleanup CleanupConfig
}

// APIConfig holds API server configuration
type APIConfig struct {
	Port         int
	Host         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
}

// RedisConfig holds Redis connection configuration
type RedisConfig struct {
	Address    string
	Password   string
	DB         int
	MaxRetries int
	PoolSize   int
}

// StorageConfig holds S3/MinIO configuration
type StorageConfig struct {
	Endpoint             string
	Region               string
	Bucket               string
	AccessKeyID          string
	SecretAccessKey      string
	PresignedURLExpiry   time.Duration
	StreamThresholdBytes int64
	UsePathStyle         bool // For MinIO
}

// ExtractorConfig holds extractor tool configuration
type ExtractorConfig struct {
	YtdlpPath         string
	FFmpegPath        string
	YtdlpTimeout      time.Duration
	FFmpegTimeout     time.Duration
	MaxConcurrentJobs int
	TempDir           string
}

// WorkerConfig holds job worker configuration
type WorkerConfig struct {
	Concurrency     int
	ShutdownTimeout time.Duration
	MaxRetries      int
	RetryDelayBase  time.Duration
}

// LoggerConfig holds logger configuration
type LoggerConfig struct {
	Level      string // debug, info, warn, error
	Format     string // json, text
	OutputPath string // stdout, file path
}

// CacheConfig holds cache configuration
type CacheConfig struct {
	Enabled bool          // Enable caching
	Address string        // Redis address (same as main Redis)
	Prefix  string        // Cache key prefix
	MaxTTL  time.Duration // Maximum TTL for any cached item
}

// CleanupConfig holds cleanup configuration
type CleanupConfig struct {
	Enabled    bool          // Enable automatic cleanup
	Interval   time.Duration // How often to run cleanup
	MaxAge     time.Duration // Delete files older than this
	MaxDirSize int64         // Max directory size before forced cleanup
	TempDirs   []string      // Directories to cleanup
	LogDir     string        // Log directory
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	cfg := &Config{
		API: APIConfig{
			Port:         getEnvInt("API_PORT", 8080),
			Host:         getEnv("API_HOST", "0.0.0.0"),
			ReadTimeout:  getEnvDuration("API_READ_TIMEOUT", 30*time.Second),
			WriteTimeout: getEnvDuration("API_WRITE_TIMEOUT", 30*time.Second),
		},
		Redis: RedisConfig{
			Address:    getEnv("REDIS_ADDR", "localhost:6379"),
			Password:   getEnv("REDIS_PASSWORD", ""),
			DB:         getEnvInt("REDIS_DB", 0),
			MaxRetries: getEnvInt("REDIS_MAX_RETRIES", 3),
			PoolSize:   getEnvInt("REDIS_POOL_SIZE", 10),
		},
		Storage: StorageConfig{
			Endpoint:             getEnv("S3_ENDPOINT", ""),
			Region:               getEnv("S3_REGION", "us-east-1"),
			Bucket:               getEnv("S3_BUCKET", "media-extraction-output"),
			AccessKeyID:          getEnv("AWS_ACCESS_KEY_ID", ""),
			SecretAccessKey:      getEnv("AWS_SECRET_ACCESS_KEY", ""),
			PresignedURLExpiry:   getEnvDuration("S3_PRESIGNED_EXPIRY", 24*time.Hour),
			StreamThresholdBytes: getEnvInt64("S3_STREAM_THRESHOLD", 500*1024*1024), // 500MB
			UsePathStyle:         getEnvBool("S3_USE_PATH_STYLE", true),             // MinIO uses path style
		},
		Extractor: ExtractorConfig{
			YtdlpPath:         getEnv("YTDLP_PATH", "yt-dlp"),
			FFmpegPath:        getEnv("FFMPEG_PATH", "ffmpeg"),
			YtdlpTimeout:      getEnvDuration("YTDLP_TIMEOUT", 10*time.Minute),
			FFmpegTimeout:     getEnvDuration("FFMPEG_TIMEOUT", 30*time.Minute),
			MaxConcurrentJobs: getEnvInt("MAX_CONCURRENT_JOBS", 8),
			TempDir:           getEnv("TEMP_DIR", os.TempDir()),
		},
		Worker: WorkerConfig{
			Concurrency:     getEnvInt("WORKER_CONCURRENCY", 8),
			ShutdownTimeout: getEnvDuration("WORKER_SHUTDOWN_TIMEOUT", 30*time.Second),
			MaxRetries:      getEnvInt("JOB_MAX_RETRIES", 3),
			RetryDelayBase:  getEnvDuration("JOB_RETRY_DELAY_BASE", 5*time.Second),
		},
		Logger: LoggerConfig{
			Level:      getEnv("LOG_LEVEL", "info"),
			Format:     getEnv("LOG_FORMAT", "json"),
			OutputPath: getEnv("LOG_OUTPUT", "stdout"),
		},
		Cache: CacheConfig{
			Enabled: getEnvBool("CACHE_ENABLED", true),
			Address: getEnv("CACHE_ADDR", "localhost:6379"), // Defaults to same Redis instance
			Prefix:  getEnv("CACHE_PREFIX", "cache:"),
			MaxTTL:  getEnvDuration("CACHE_MAX_TTL", 30*24*time.Hour), // 30 days
		},
		Cleanup: CleanupConfig{
			Enabled:    getEnvBool("CLEANUP_ENABLED", true),
			Interval:   getEnvDuration("CLEANUP_INTERVAL", 24*time.Hour),       // Daily
			MaxAge:     getEnvDuration("CLEANUP_MAX_AGE", 7*24*time.Hour),      // 7 days
			MaxDirSize: getEnvInt64("CLEANUP_MAX_DIR_SIZE", 10*1024*1024*1024), // 10GB
			TempDirs: []string{
				getEnv("CLEANUP_TEMP_DIR", "./tmp"),
			},
			LogDir: getEnv("CLEANUP_LOG_DIR", "./logs"),
		},
	}

	// Validate critical configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("configuration validation failed: %w", err)
	}

	return cfg, nil
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Redis.Address == "" {
		return fmt.Errorf("REDIS_ADDR is required")
	}

	if c.Storage.Bucket == "" {
		return fmt.Errorf("S3_BUCKET is required")
	}

	if c.Extractor.YtdlpPath == "" {
		return fmt.Errorf("YTDLP_PATH is required")
	}

	if c.Extractor.FFmpegPath == "" {
		return fmt.Errorf("FFMPEG_PATH is required")
	}

	if c.API.Port < 1 || c.API.Port > 65535 {
		return fmt.Errorf("API_PORT must be between 1 and 65535, got %d", c.API.Port)
	}

	if c.Worker.Concurrency < 1 {
		return fmt.Errorf("WORKER_CONCURRENCY must be >= 1")
	}

	return nil
}

// Helper functions
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.ParseInt(value, 10, 64); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		return value == "true" || value == "1" || value == "yes"
	}
	return defaultValue
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
