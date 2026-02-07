package middleware

import (
	"github.com/gofiber/fiber/v2"
)

// APIKeyAuth checks for valid API key (header only for security)
func APIKeyAuth(validKey string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Get API key from header only (never from URL/query to avoid logging in browser history)
		apiKey := c.Get("X-API-Key")

		// Validate API key
		if apiKey == "" || apiKey != validKey {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid or missing API key",
				"hint":  "Provide X-API-Key header with your API key",
			})
		}

		return c.Next()
	}
}
