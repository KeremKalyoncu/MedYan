package middleware

import (
	"github.com/gofiber/fiber/v2"
)

// APIKeyAuth checks for valid API key
func APIKeyAuth(validKey string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		// Check header first
		apiKey := c.Get("X-API-Key")

		// If not in header, check query parameter
		if apiKey == "" {
			apiKey = c.Query("api_key")
		}

		// If still empty or invalid
		if apiKey == "" || apiKey != validKey {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "Invalid or missing API key",
				"hint":  "Provide X-API-Key header or api_key query parameter",
			})
		}

		return c.Next()
	}
}
