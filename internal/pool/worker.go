package pool

import (
	"context"
	"sync"
	"sync/atomic"
)

// WorkerPool manages a pool of worker goroutines for parallel task processing
type WorkerPool struct {
	workerCount int
	taskQueue   chan Task
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
	activeJobs  int64
}

// Task represents a unit of work to be processed by the worker pool
type Task func(ctx context.Context) error

// NewWorkerPool creates a new worker pool with the specified number of workers
func NewWorkerPool(workerCount int, queueSize int) *WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())

	pool := &WorkerPool{
		workerCount: workerCount,
		taskQueue:   make(chan Task, queueSize),
		ctx:         ctx,
		cancel:      cancel,
	}

	// Start workers
	for i := 0; i < workerCount; i++ {
		pool.wg.Add(1)
		go pool.worker(i)
	}

	return pool
}

// worker processes tasks from the queue
func (wp *WorkerPool) worker(id int) {
	defer wp.wg.Done()

	for {
		select {
		case <-wp.ctx.Done():
			return
		case task, ok := <-wp.taskQueue:
			if !ok {
				return
			}

			atomic.AddInt64(&wp.activeJobs, 1)
			_ = task(wp.ctx) // Execute task (error handling is task's responsibility)
			atomic.AddInt64(&wp.activeJobs, -1)
		}
	}
}

// Submit adds a task to the worker pool queue (non-blocking)
func (wp *WorkerPool) Submit(task Task) bool {
	select {
	case wp.taskQueue <- task:
		return true
	case <-wp.ctx.Done():
		return false
	default:
		return false // Queue full
	}
}

// SubmitWait adds a task and waits if queue is full (blocking)
func (wp *WorkerPool) SubmitWait(ctx context.Context, task Task) error {
	select {
	case wp.taskQueue <- task:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-wp.ctx.Done():
		return wp.ctx.Err()
	}
}

// ActiveJobs returns the current number of active jobs being processed
func (wp *WorkerPool) ActiveJobs() int64 {
	return atomic.LoadInt64(&wp.activeJobs)
}

// Shutdown gracefully stops the worker pool
func (wp *WorkerPool) Shutdown() {
	close(wp.taskQueue)
	wp.cancel()
	wp.wg.Wait()
}

// ShutdownWithContext stops the worker pool with a timeout
func (wp *WorkerPool) ShutdownWithContext(ctx context.Context) error {
	close(wp.taskQueue)

	done := make(chan struct{})
	go func() {
		wp.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		wp.cancel()
		return nil
	case <-ctx.Done():
		wp.cancel()
		return ctx.Err()
	}
}
