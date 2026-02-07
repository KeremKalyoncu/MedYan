package middleware

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/gsker/media-extraction-saas/internal/errors"
	"github.com/gsker/media-extraction-saas/internal/types"
	"go.uber.org/zap"
)

// ValidateExtractionRequest validates an ExtractionRequest
func ValidateExtractionRequest(req *types.ExtractionRequest) error {
	// Validate URL
	if req.URL == "" {
		return errors.ErrInvalidURL.WithDetails("URL is required")
	}

	// Basic URL validation
	if !isValidURL(req.URL) {
		return errors.ErrInvalidURL.WithDetails(fmt.Sprintf("URL is not valid: %s", req.URL))
	}

	// Validate quality if provided
	if req.Quality != "" {
		if _, exists := types.QualityPresets[req.Quality]; !exists {
			validQualifies := strings.Join(getValidQualities(), ", ")
			return errors.ErrInvalidQuality.WithDetails(
				fmt.Sprintf("Quality must be one of: %s", validQualifies),
			)
		}
	}

	// Validate format if provided
	if req.Format != "" {
		if !isValidFormat(req.Format) {
			validFormats := strings.Join(getValidFormats(), ", ")
			return errors.ErrInvalidFormat.WithDetails(
				fmt.Sprintf("Format must be one of: %s", validFormats),
			)
		}
	}

	// Validate audio format if extract_audio is true
	if req.ExtractAudio && req.AudioFormat != "" {
		if !isValidAudioFormat(req.AudioFormat) {
			validFormats := strings.Join(getValidAudioFormats(), ", ")
			return errors.ErrInvalidFormat.WithDetails(
				fmt.Sprintf("Audio format must be one of: %s", validFormats),
			)
		}
	}

	return nil
}

// ErrorHandlingMiddleware handles panics and converts errors to proper responses
func ErrorHandlingMiddleware(logger *zap.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Panic recovered",
					zap.Any("panic", r),
					zap.String("path", c.Path()),
					zap.String("method", c.Method()),
				)
				_ = c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": "Internal server error",
					"code":  "INTERNAL_ERROR",
				})
			}
		}()

		err := c.Next()
		if err != nil {
			return handleError(c, err, logger)
		}
		return nil
	}
}

// handleError converts errors to proper HTTP responses
func handleError(c *fiber.Ctx, err error, logger *zap.Logger) error {
	statusCode := errors.GetStatusCode(err)
	errorCode := errors.GetErrorCode(err)
	errorMessage := errors.GetErrorMessage(err)

	// Log the error
	logger.Error("Request error",
		zap.Error(err),
		zap.String("path", c.Path()),
		zap.String("method", c.Method()),
		zap.Int("status", statusCode),
		zap.String("error_code", errorCode),
	)

	return c.Status(statusCode).JSON(fiber.Map{
		"error": errorMessage,
		"code":  errorCode,
		"path":  c.Path(),
	})
}

// ValidationMiddleware validates common request patterns
func ValidationMiddleware(logger *zap.Logger) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Check content type for POST/PUT requests
		if c.Method() == "POST" || c.Method() == "PUT" {
			if ct := c.Get("Content-Type"); !strings.Contains(ct, "application/json") {
				return errors.ErrInvalidRequest.WithDetails("Content-Type must be application/json")
			}
		}

		return c.Next()
	}
}

// Validation helper functions
func isValidURL(urlStr string) bool {
	_, err := url.ParseRequestURI(urlStr)
	return err == nil
}

func isValidFormat(format string) bool {
	validFormats := map[string]bool{
		"mp4":  true,
		"webm": true,
		"mkv":  true,
		"avi":  true,
		"mov":  true,
		"flv":  true,
	}
	return validFormats[strings.ToLower(format)]
}

func isValidAudioFormat(format string) bool {
	validFormats := map[string]bool{
		"mp3":    true,
		"aac":    true,
		"flac":   true,
		"m4a":    true,
		"opus":   true,
		"vorbis": true,
	}
	return validFormats[strings.ToLower(format)]
}

func getValidFormats() []string {
	return []string{"mp4", "webm", "mkv", "avi", "mov", "flv"}
}

func getValidAudioFormats() []string {
	return []string{"mp3", "aac", "flac", "m4a", "opus", "vorbis"}
}

func getValidQualities() []string {
	qualities := make([]string, 0)
	for quality := range types.QualityPresets {
		qualities = append(qualities, quality)
	}
	return qualities
}
