package metrics

import (
	"sync"
	"sync/atomic"
	"time"
)

// Metrics collects application performance metrics
type Metrics struct {
	// Request metrics
	TotalRequests  atomic.Uint64
	SuccessfulJobs atomic.Uint64
	FailedJobs     atomic.Uint64
	ActiveJobs     atomic.Int64

	// Performance metrics
	AverageJobDuration atomic.Int64 // microseconds
	TotalDownloadedMB  atomic.Uint64

	// System metrics
	Uptime      time.Time
	TotalErrors atomic.Uint64

	// Per-platform metrics
	platformStats sync.Map // platform -> *PlatformStats
}

// PlatformStats tracks metrics per platform
type PlatformStats struct {
	TotalJobs      atomic.Uint64
	SuccessfulJobs atomic.Uint64
	FailedJobs     atomic.Uint64
}

// Global metrics instance
var globalMetrics *Metrics

func init() {
	globalMetrics = &Metrics{
		Uptime: time.Now(),
	}
}

// GetMetrics returns the global metrics instance
func GetMetrics() *Metrics {
	return globalMetrics
}

// IncrementRequests increments total request counter
func (m *Metrics) IncrementRequests() {
	m.TotalRequests.Add(1)
}

// RecordJobSuccess records a successful job
func (m *Metrics) RecordJobSuccess(platform string, duration time.Duration, sizeMB uint64) {
	m.SuccessfulJobs.Add(1)
	m.ActiveJobs.Add(-1)
	m.TotalDownloadedMB.Add(sizeMB)

	// Update average duration (simple moving average)
	m.AverageJobDuration.Store(duration.Microseconds())

	// Update platform stats
	m.updatePlatformStats(platform, true)
}

// RecordJobFailure records a failed job
func (m *Metrics) RecordJobFailure(platform string) {
	m.FailedJobs.Add(1)
	m.ActiveJobs.Add(-1)
	m.TotalErrors.Add(1)

	// Update platform stats
	m.updatePlatformStats(platform, false)
}

// RecordJobStart records job start
func (m *Metrics) RecordJobStart(platform string) {
	m.ActiveJobs.Add(1)

	// Ensure platform stats exist
	if _, exists := m.platformStats.Load(platform); !exists {
		m.platformStats.Store(platform, &PlatformStats{})
	}

	if stats, ok := m.platformStats.Load(platform); ok {
		stats.(*PlatformStats).TotalJobs.Add(1)
	}
}

// updatePlatformStats updates per-platform statistics
func (m *Metrics) updatePlatformStats(platform string, success bool) {
	statsInterface, _ := m.platformStats.LoadOrStore(platform, &PlatformStats{})
	stats := statsInterface.(*PlatformStats)

	if success {
		stats.SuccessfulJobs.Add(1)
	} else {
		stats.FailedJobs.Add(1)
	}
}

// GetSnapshot returns current metrics snapshot
func (m *Metrics) GetSnapshot() map[string]interface{} {
	uptime := time.Since(m.Uptime)

	// Calculate success rate
	total := m.SuccessfulJobs.Load() + m.FailedJobs.Load()
	successRate := float64(0)
	if total > 0 {
		successRate = float64(m.SuccessfulJobs.Load()) / float64(total) * 100
	}

	snapshot := map[string]interface{}{
		"uptime_seconds":      int64(uptime.Seconds()),
		"total_requests":      m.TotalRequests.Load(),
		"successful_jobs":     m.SuccessfulJobs.Load(),
		"failed_jobs":         m.FailedJobs.Load(),
		"active_jobs":         m.ActiveJobs.Load(),
		"success_rate":        successRate,
		"avg_job_duration_ms": m.AverageJobDuration.Load() / 1000,
		"total_downloaded_mb": m.TotalDownloadedMB.Load(),
		"total_errors":        m.TotalErrors.Load(),
		"platforms":           m.getPlatformSnapshot(),
	}

	return snapshot
}

// getPlatformSnapshot returns platform-specific metrics
func (m *Metrics) getPlatformSnapshot() map[string]interface{} {
	platforms := make(map[string]interface{})

	m.platformStats.Range(func(key, value interface{}) bool {
		platform := key.(string)
		stats := value.(*PlatformStats)

		total := stats.TotalJobs.Load()
		successRate := float64(0)
		if total > 0 {
			successRate = float64(stats.SuccessfulJobs.Load()) / float64(total) * 100
		}

		platforms[platform] = map[string]interface{}{
			"total_jobs":      total,
			"successful_jobs": stats.SuccessfulJobs.Load(),
			"failed_jobs":     stats.FailedJobs.Load(),
			"success_rate":    successRate,
		}

		return true
	})

	return platforms
}
