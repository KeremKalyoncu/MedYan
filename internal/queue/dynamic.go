package queue

import (
	"runtime"
	"sync"
	"time"

	"go.uber.org/zap"
)

// DynamicConcurrency adjusts worker concurrency based on system load
// This prevents resource exhaustion during high traffic
type DynamicConcurrency struct {
	minWorkers     int
	maxWorkers     int
	currentWorkers int
	targetCPU      float64 // Target CPU utilization (0.0 - 1.0)
	interval       time.Duration
	logger         *zap.Logger
	mu             sync.RWMutex
	closeCh        chan struct{}
}

// NewDynamicConcurrency creates a dynamic concurrency controller
// minWorkers: Minimum concurrent workers (e.g., 2)
// maxWorkers: Maximum concurrent workers (e.g., 12)
// targetCPU: Target CPU utilization 0.7 = 70% (scale up if under, down if over)
func NewDynamicConcurrency(minWorkers, maxWorkers int, targetCPU float64, logger *zap.Logger) *DynamicConcurrency {
	return &DynamicConcurrency{
		minWorkers:     minWorkers,
		maxWorkers:     maxWorkers,
		currentWorkers: minWorkers,
		targetCPU:      targetCPU,
		interval:       30 * time.Second, // Adjust every 30 seconds
		logger:         logger,
		closeCh:        make(chan struct{}),
	}
}

// Start begins monitoring and adjusting concurrency
func (dc *DynamicConcurrency) Start() {
	go dc.monitor()
}

// Stop stops concurrency monitoring
func (dc *DynamicConcurrency) Stop() {
	close(dc.closeCh)
}

// GetConcurrency returns current worker count
func (dc *DynamicConcurrency) GetConcurrency() int {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.currentWorkers
}

func (dc *DynamicConcurrency) monitor() {
	ticker := time.NewTicker(dc.interval)
	defer ticker.Stop()

	var prevIdleTime, prevTotalTime uint64

	for {
		select {
		case <-ticker.C:
			cpuUsage := dc.getCPUUsage(&prevIdleTime, &prevTotalTime)
			dc.adjust(cpuUsage)
		case <-dc.closeCh:
			return
		}
	}
}

func (dc *DynamicConcurrency) getCPUUsage(prevIdle, prevTotal *uint64) float64 {
	// Simple CPU estimation based on goroutine count and NumCPU
	numCPU := runtime.NumCPU()
	numGoroutine := runtime.NumGoroutine()

	// Estimate: high goroutine count relative to CPU = high load
	estimatedUsage := float64(numGoroutine) / float64(numCPU*100)
	if estimatedUsage > 1.0 {
		estimatedUsage = 0.95 // Cap at 95%
	}

	return estimatedUsage
}

func (dc *DynamicConcurrency) adjust(cpuUsage float64) {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	oldWorkers := dc.currentWorkers

	// Scale up if CPU below target (more capacity available)
	if cpuUsage < dc.targetCPU*0.8 && dc.currentWorkers < dc.maxWorkers {
		dc.currentWorkers++
		dc.logger.Info("Scaling up workers",
			zap.Int("old_workers", oldWorkers),
			zap.Int("new_workers", dc.currentWorkers),
			zap.Float64("cpu_usage", cpuUsage),
		)
	}

	// Scale down if CPU above target (overloaded)
	if cpuUsage > dc.targetCPU*1.2 && dc.currentWorkers > dc.minWorkers {
		dc.currentWorkers--
		dc.logger.Info("Scaling down workers",
			zap.Int("old_workers", oldWorkers),
			zap.Int("new_workers", dc.currentWorkers),
			zap.Float64("cpu_usage", cpuUsage),
		)
	}
}

// Stats returns current concurrency statistics
func (dc *DynamicConcurrency) Stats() map[string]interface{} {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	return map[string]interface{}{
		"current_workers": dc.currentWorkers,
		"min_workers":     dc.minWorkers,
		"max_workers":     dc.maxWorkers,
		"target_cpu":      dc.targetCPU,
		"num_cpu":         runtime.NumCPU(),
		"num_goroutine":   runtime.NumGoroutine(),
	}
}
