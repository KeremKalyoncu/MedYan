package dedup

import (
	"context"
	"errors"
	"sync"
	"time"
)

// Singleflight prevents duplicate function calls for the same key
// If multiple requests come for the same URL, only one will execute
// and all others will wait for and receive the same result
type Singleflight struct {
	mu    sync.Mutex
	calls map[string]*call
}

// call represents an in-flight or completed Do call
type call struct {
	wg  sync.WaitGroup
	val interface{}
	err error

	// For timeout and cleanup
	deadline time.Time
}

// Result represents the result of a Do call
type Result struct {
	Val    interface{}
	Err    error
	Shared bool // Whether the result was shared with other callers
}

// NewSingleflight creates a new Singleflight instance
func NewSingleflight() *Singleflight {
	sf := &Singleflight{
		calls: make(map[string]*call),
	}

	// Start cleanup goroutine
	go sf.cleanup()

	return sf
}

// Do executes and returns the results of the given function, ensuring that
// only one execution is in-flight for a given key at a time. If a duplicate
// comes in, the duplicate caller waits for the original to complete and
// receives the same results.
func (sf *Singleflight) Do(key string, fn func() (interface{}, error)) Result {
	sf.mu.Lock()

	if c, ok := sf.calls[key]; ok {
		// Another goroutine is already executing this key
		sf.mu.Unlock()
		c.wg.Wait() // Wait for it to finish
		return Result{Val: c.val, Err: c.err, Shared: true}
	}

	// First caller for this key - create new call
	c := &call{
		deadline: time.Now().Add(5 * time.Minute), // Cleanup after 5 min
	}
	c.wg.Add(1)
	sf.calls[key] = c
	sf.mu.Unlock()

	// Execute the function
	c.val, c.err = fn()
	c.wg.Done()

	// Cleanup immediately after completion
	sf.mu.Lock()
	delete(sf.calls, key)
	sf.mu.Unlock()

	return Result{Val: c.val, Err: c.err, Shared: false}
}

// DoContext is like Do but respects context cancellation
func (sf *Singleflight) DoContext(ctx context.Context, key string, fn func() (interface{}, error)) Result {
	// Check if context is already cancelled
	select {
	case <-ctx.Done():
		return Result{Err: ctx.Err(), Shared: false}
	default:
	}

	sf.mu.Lock()

	if c, ok := sf.calls[key]; ok {
		// Another goroutine is already executing this key
		sf.mu.Unlock()

		// Wait for either completion or context cancellation
		done := make(chan struct{})
		go func() {
			c.wg.Wait()
			close(done)
		}()

		select {
		case <-done:
			return Result{Val: c.val, Err: c.err, Shared: true}
		case <-ctx.Done():
			return Result{Err: ctx.Err(), Shared: true}
		}
	}

	// First caller for this key
	c := &call{
		deadline: time.Now().Add(5 * time.Minute),
	}
	c.wg.Add(1)
	sf.calls[key] = c
	sf.mu.Unlock()

	// Execute function in goroutine so we can handle context cancellation
	resultCh := make(chan struct{})
	go func() {
		c.val, c.err = fn()
		c.wg.Done()
		close(resultCh)
	}()

	// Wait for either completion or context cancellation
	select {
	case <-resultCh:
		// Cleanup
		sf.mu.Lock()
		delete(sf.calls, key)
		sf.mu.Unlock()
		return Result{Val: c.val, Err: c.err, Shared: false}
	case <-ctx.Done():
		// Context cancelled - but don't cancel the underlying work
		// Other waiters might still want it
		return Result{Err: ctx.Err(), Shared: false}
	}
}

// Forget removes a key from the in-flight calls map
// This is useful if you want to force a retry
func (sf *Singleflight) Forget(key string) {
	sf.mu.Lock()
	delete(sf.calls, key)
	sf.mu.Unlock()
}

// cleanup periodically removes stale entries (defensive)
func (sf *Singleflight) cleanup() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		sf.mu.Lock()
		for key, c := range sf.calls {
			if now.After(c.deadline) {
				delete(sf.calls, key)
			}
		}
		sf.mu.Unlock()
	}
}

// Stats returns statistics about in-flight calls
func (sf *Singleflight) Stats() map[string]interface{} {
	sf.mu.Lock()
	defer sf.mu.Unlock()

	return map[string]interface{}{
		"in_flight_calls": len(sf.calls),
	}
}

// ErrDuplicate indicates that a call was deduplicated (not an actual error)
var ErrDuplicate = errors.New("request was deduplicated")
