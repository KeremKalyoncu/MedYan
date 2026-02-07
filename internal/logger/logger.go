package logger

import (
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Config holds logger configuration
type Config struct {
	Level         string // debug, info, warn, error
	FileName      string // Log file path
	MaxSize       int    // Max size in MB before rotation
	MaxBackups    int    // Max number of old log files to retain
	MaxAge        int    // Max days to retain files
	Compress      bool   // Whether to compress old files
	Format        string // json or text
	ConsoleOutput bool   // Also output to console
}

// New creates a configured logger with file rotation support
func New(cfg Config) (*zap.Logger, error) {
	// Create logs directory if needed
	if cfg.FileName != "" {
		logDir := filepath.Dir(cfg.FileName)
		if err := os.MkdirAll(logDir, 0755); err != nil {
			return nil, err
		}
	}

	// Configure log level
	var level zapcore.Level
	switch cfg.Level {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warn":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	default:
		level = zapcore.InfoLevel
	}

	// Create encoder config
	encoderCfg := zapcore.EncoderConfig{
		TimeKey:        "timestamp",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    "function",
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// Create appropriate encoder
	var encoder zapcore.Encoder
	if cfg.Format == "text" {
		encoder = zapcore.NewConsoleEncoder(encoderCfg)
	} else {
		encoder = zapcore.NewJSONEncoder(encoderCfg)
	}

	// Create core writers
	var cores []zapcore.Core

	// File writer with rotation
	if cfg.FileName != "" {
		fileWriter := &lumberjack.Logger{
			Filename:   cfg.FileName,
			MaxSize:    cfg.MaxSize,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge,
			Compress:   cfg.Compress,
		}

		fileCore := zapcore.NewCore(encoder, zapcore.AddSync(fileWriter), level)
		cores = append(cores, fileCore)
	}

	// Console writer
	if cfg.ConsoleOutput {
		consoleCore := zapcore.NewCore(encoder, zapcore.AddSync(os.Stdout), level)
		cores = append(cores, consoleCore)
	}

	// Combine cores
	combined := zapcore.NewTee(cores...)

	// Create logger
	logger := zap.New(
		combined,
		zap.AddCaller(),
		zap.AddCallerSkip(1),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)

	return logger, nil
}

// NewProduction creates a production logger similar to zap.NewProduction
// but with file rotation support
func NewProduction(logDir string) (*zap.Logger, error) {
	cfg := Config{
		Level:         "info",
		FileName:      filepath.Join(logDir, "app.log"),
		MaxSize:       100, // 100MB
		MaxBackups:    5,
		MaxAge:        30, // 30 days
		Compress:      true,
		Format:        "json",
		ConsoleOutput: false,
	}

	return New(cfg)
}

// NewDevelopment creates a development logger
func NewDevelopment() (*zap.Logger, error) {
	cfg := Config{
		Level:         "debug",
		FileName:      "",
		Format:        "text",
		ConsoleOutput: true,
	}

	return New(cfg)
}
