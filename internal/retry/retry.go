package retry

import (
	"context"
	"errors"
	"math"
	"math/rand"
	"time"
)

var (
	// ErrMaxAttemptsReached is returned when all retry attempts are exhausted
	ErrMaxAttemptsReached = errors.New("maximum retry attempts reached")
)

// Config holds retry configuration
type Config struct {
	// MaxAttempts is the maximum number of attempts (including initial attempt)
	MaxAttempts int
	// InitialDelay is the delay before first retry
	InitialDelay time.Duration
	// MaxDelay is the maximum delay between retries
	MaxDelay time.Duration
	// Multiplier is the backoff multiplier (typically 2 for exponential backoff)
	Multiplier float64
	// Jitter adds randomness to backoff to prevent thundering herd
	// Jitter = 0.0 (no jitter) to 1.0 (full jitter)
	Jitter float64
	// RetryableErrors returns true if error should be retried
	RetryableErrors func(err error) bool
	// OnRetry is called before each retry attempt
	OnRetry func(attempt int, delay time.Duration, err error)
}

// DefaultConfig returns sensible default retry configuration
func DefaultConfig() Config {
	return Config{
		MaxAttempts:  3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     10 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.3, // 30% jitter
		RetryableErrors: func(err error) bool {
			// By default, retry all errors
			return err != nil
		},
	}
}

// Retry executes fn with exponential backoff retry logic
func Retry(ctx context.Context, config Config, fn func() error) error {
	if config.MaxAttempts <= 0 {
		config.MaxAttempts = 1
	}

	if config.InitialDelay == 0 {
		config.InitialDelay = 100 * time.Millisecond
	}

	if config.MaxDelay == 0 {
		config.MaxDelay = 30 * time.Second
	}

	if config.Multiplier == 0 {
		config.Multiplier = 2.0
	}

	if config.Jitter < 0 {
		config.Jitter = 0
	}
	if config.Jitter > 1 {
		config.Jitter = 1
	}

	if config.RetryableErrors == nil {
		config.RetryableErrors = func(err error) bool {
			return err != nil
		}
	}

	var lastErr error

	for attempt := 1; attempt <= config.MaxAttempts; attempt++ {
		// Check context before attempting
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Execute function
		err := fn()

		// Success
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !config.RetryableErrors(err) {
			return err
		}

		// Last attempt - don't retry
		if attempt >= config.MaxAttempts {
			break
		}

		// Calculate delay with exponential backoff
		delay := calculateDelay(attempt, config)

		// Call onRetry callback
		if config.OnRetry != nil {
			config.OnRetry(attempt, delay, err)
		}

		// Wait before retry
		select {
		case <-time.After(delay):
			// Continue to next attempt
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	// All attempts exhausted
	return lastErr
}

// calculateDelay calculates delay with exponential backoff and jitter
func calculateDelay(attempt int, config Config) time.Duration {
	// Exponential backoff: initialDelay * multiplier^(attempt-1)
	backoff := float64(config.InitialDelay) * math.Pow(config.Multiplier, float64(attempt-1))

	// Cap at max delay
	if backoff > float64(config.MaxDelay) {
		backoff = float64(config.MaxDelay)
	}

	// Apply jitter
	if config.Jitter > 0 {
		// Full jitter: random between 0 and backoff
		// Partial jitter: random between (1-jitter)*backoff and backoff
		jitterAmount := backoff * config.Jitter
		backoff = backoff - jitterAmount + (rand.Float64() * jitterAmount * 2)
	}

	return time.Duration(backoff)
}

// RetryWithContext is a convenience function with context
func RetryWithContext(ctx context.Context, maxAttempts int, initialDelay time.Duration, fn func() error) error {
	config := DefaultConfig()
	config.MaxAttempts = maxAttempts
	config.InitialDelay = initialDelay
	return Retry(ctx, config, fn)
}

// RetryWithBackoff executes with custom backoff parameters
func RetryWithBackoff(ctx context.Context, maxAttempts int, initialDelay, maxDelay time.Duration, multiplier float64, fn func() error) error {
	config := DefaultConfig()
	config.MaxAttempts = maxAttempts
	config.InitialDelay = initialDelay
	config.MaxDelay = maxDelay
	config.Multiplier = multiplier
	return Retry(ctx, config, fn)
}

// Do is a simple retry function with default config
func Do(ctx context.Context, fn func() error) error {
	return Retry(ctx, DefaultConfig(), fn)
}
