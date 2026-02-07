package middleware

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/compress"
)

// CompressionMiddleware returns a Fiber middleware for response compression
// This significantly reduces bandwidth usage and improves response times
func CompressionMiddleware() fiber.Handler {
	return compress.New(compress.Config{
		// Compression level (0-9, where 9 is max compression)
		// Level 6 offers good balance between speed and compression ratio
		Level: compress.LevelBestSpeed, // Level 1 - prioritize speed over compression ratio

		// Only compress responses larger than 1KB
		// Small responses have negligible benefit from compression
		Next: func(c *fiber.Ctx) bool {
			// Skip compression for /downloads/* (already compressed media files)
			if len(c.Path()) > 10 && c.Path()[:10] == "/downloads" {
				return true
			}

			// Skip compression for already compressed content types
			contentType := string(c.Response().Header.ContentType())
			if isCompressedContentType(contentType) {
				return true
			}

			return false
		},
	})
}

// isCompressedContentType checks if content type is already compressed
func isCompressedContentType(contentType string) bool {
	compressedTypes := []string{
		"video/",
		"audio/",
		"image/jpeg",
		"image/png",
		"image/gif",
		"image/webp",
		"application/zip",
		"application/gzip",
		"application/x-gzip",
		"application/x-compress",
		"application/x-compressed",
	}

	for _, ct := range compressedTypes {
		if len(contentType) >= len(ct) && contentType[:len(ct)] == ct {
			return true
		}
	}

	return false
}
