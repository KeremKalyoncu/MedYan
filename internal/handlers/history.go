package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gofiber/fiber/v2"
	"go.uber.org/zap"

	"github.com/KeremKalyoncu/MedYan/internal/queue"
	"github.com/KeremKalyoncu/MedYan/internal/types"
)

// HistoryHandler manages per-site download history
type HistoryHandler struct {
	queueClient *queue.Client
	logger      *zap.Logger
}

// NewHistoryHandler creates a new history handler
func NewHistoryHandler(queueClient *queue.Client, logger *zap.Logger) *HistoryHandler {
	return &HistoryHandler{
		queueClient: queueClient,
		logger:      logger,
	}
}

// AddToHistory adds download to site history (max 20 items, 24h TTL)
// POST /api/history
func (h *HistoryHandler) AddToHistory(c *fiber.Ctx) error {
	var req types.HistoryItem
	if err := c.BodyParser(&req); err != nil {
		return c.Status(400).JSON(fiber.Map{"error": "Invalid request"})
	}

	if req.URL == "" {
		return c.Status(400).JSON(fiber.Map{"error": "URL required"})
	}

	// Set timestamp if not provided
	if req.Timestamp == 0 {
		req.Timestamp = time.Now().Unix()
	}

	// Get site hostname
	hostname := c.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	historyKey := fmt.Sprintf("site:%s:history", hostname)

	// Load existing history
	var history []types.HistoryItem
	if historyJSON, err := h.queueClient.GetRedis().Get(context.Background(), historyKey).Result(); err == nil {
		json.Unmarshal([]byte(historyJSON), &history)
	}

	// Add new item to front (newest first)
	history = append([]types.HistoryItem{req}, history...)

	// Keep only 20 items
	if len(history) > 20 {
		history = history[:20]
	}

	// Save to Redis with 24h TTL
	historyData, _ := json.Marshal(history)
	h.queueClient.GetRedis().Set(context.Background(), historyKey, string(historyData), 24*time.Hour)

	h.logger.Info("Added to history", zap.String("hostname", hostname), zap.String("title", req.Title))

	return c.JSON(fiber.Map{"success": true})
}

// GetHistory retrieves site's download history
// GET /api/history
func (h *HistoryHandler) GetHistory(c *fiber.Ctx) error {
	hostname := c.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	historyKey := fmt.Sprintf("site:%s:history", hostname)

	var history []types.HistoryItem
	if historyJSON, err := h.queueClient.GetRedis().Get(context.Background(), historyKey).Result(); err == nil {
		json.Unmarshal([]byte(historyJSON), &history)
	}

	return c.JSON(fiber.Map{
		"history": history,
		"count":   len(history),
	})
}

// ClearHistory clears site's download history
// DELETE /api/history
func (h *HistoryHandler) ClearHistory(c *fiber.Ctx) error {
	hostname := c.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	historyKey := fmt.Sprintf("site:%s:history", hostname)
	h.queueClient.GetRedis().Del(context.Background(), historyKey)

	h.logger.Info("Cleared history", zap.String("hostname", hostname))

	return c.JSON(fiber.Map{"success": true, "message": "History cleared"})
}
