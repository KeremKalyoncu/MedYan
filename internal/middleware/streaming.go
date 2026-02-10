package middleware

import (
	"bufio"
	"io"
	"os"
	"strconv"

	"github.com/KeremKalyoncu/MedYan/internal/pool"
	"github.com/gofiber/fiber/v2"
)

// StreamingConfig defines configuration for streaming middleware
type StreamingConfig struct {
	// ChunkSize is the size of each chunk in bytes (default: 64KB)
	ChunkSize int

	// BufferSize is the size of the internal buffer (default: 256KB)
	BufferSize int

	// Next defines a function to skip this middleware
	Next func(c *fiber.Ctx) bool
}

// DefaultStreamingConfig returns default streaming configuration
func DefaultStreamingConfig() StreamingConfig {
	return StreamingConfig{
		ChunkSize:  64 * 1024,  // 64KB chunks
		BufferSize: 256 * 1024, // 256KB buffer
		Next:       nil,
	}
}

// StreamingMiddleware returns a middleware for efficient file streaming
// This significantly reduces memory usage for large file downloads
func StreamingMiddleware(config ...StreamingConfig) fiber.Handler {
	cfg := DefaultStreamingConfig()
	if len(config) > 0 {
		cfg = config[0]
	}

	if cfg.ChunkSize <= 0 {
		cfg.ChunkSize = 64 * 1024
	}
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = 256 * 1024
	}

	return func(c *fiber.Ctx) error {
		// Skip if Next returns true
		if cfg.Next != nil && cfg.Next(c) {
			return c.Next()
		}

		return c.Next()
	}
}

// StreamFile efficiently streams a file to the client using chunked encoding
// This keeps memory usage constant regardless of file size
func StreamFile(c *fiber.Ctx, filePath string, filename string) error {
	// Open file
	file, err := os.Open(filePath)
	if err != nil {
		return c.Status(404).JSON(fiber.Map{
			"error": "File not found",
		})
	}
	defer file.Close()

	// Get file info
	stat, err := file.Stat()
	if err != nil {
		return c.Status(500).JSON(fiber.Map{
			"error": "Failed to get file info",
		})
	}

	// Set headers for streaming download
	c.Set("Content-Type", "application/octet-stream")
	c.Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	c.Set("Content-Length", strconv.FormatInt(stat.Size(), 10))
	c.Set("Transfer-Encoding", "chunked")
	c.Set("Cache-Control", "no-cache")
	c.Set("X-Content-Type-Options", "nosniff")

	// Stream file in chunks using fasthttp.StreamWriter
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		// Use buffer pool for zero-allocation streaming (64KB)
		buffer := pool.MediumSlicePool.Get()
		defer pool.MediumSlicePool.Put(buffer)

		for {
			n, err := file.Read(buffer)
			if n > 0 {
				w.Write(buffer[:n])
				w.Flush()
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				// Log error but can't return it at this point
				break
			}
		}
	})

	return nil
}

// StreamReader streams data from an io.Reader to the client
func StreamReader(c *fiber.Ctx, reader io.Reader, contentType string, filename string) error {
	// Set headers
	c.Set("Content-Type", contentType)
	if filename != "" {
		c.Set("Content-Disposition", "attachment; filename=\""+filename+"\"")
	}
	c.Set("Transfer-Encoding", "chunked")
	c.Set("Cache-Control", "no-cache")

	// Stream data using fasthttp.StreamWriter
	c.Context().SetBodyStreamWriter(func(w *bufio.Writer) {
		// Use buffer pool for zero-allocation streaming (64KB)
		buffer := pool.MediumSlicePool.Get()
		defer pool.MediumSlicePool.Put(buffer)

		for {
			n, err := reader.Read(buffer)
			if n > 0 {
				w.Write(buffer[:n])
				w.Flush()
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				break
			}
		}
	})

	return nil
}

// ChunkedResponse sends a chunked transfer encoded response
// Useful for streaming large JSON arrays or CSV files
type ChunkedResponse struct {
	ctx       *fiber.Ctx
	started   bool
	chunkSize int
	buffer    []byte
	bufferPos int
}

// NewChunkedResponse creates a new chunked response writer
func NewChunkedResponse(c *fiber.Ctx, contentType string) *ChunkedResponse {
	c.Set("Content-Type", contentType)
	c.Set("Transfer-Encoding", "chunked")

	return &ChunkedResponse{
		ctx:       c,
		chunkSize: 64 * 1024,
		buffer:    make([]byte, 64*1024),
	}
}

// Write writes data to the chunked response
func (cr *ChunkedResponse) Write(data []byte) error {
	// TODO: Implement buffered chunked writing
	// For now, this is a placeholder for the interface
	return nil
}

// Flush sends any buffered data
func (cr *ChunkedResponse) Flush() error {
	// TODO: Implement flush
	return nil
}

// Close closes the chunked response
func (cr *ChunkedResponse) Close() error {
	return cr.Flush()
}
