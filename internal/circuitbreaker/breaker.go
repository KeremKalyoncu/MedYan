package circuitbreaker

import (
	"context"
	"errors"
	"sync"
	"time"
)

var (
	// ErrCircuitOpen is returned when circuit breaker is open
	ErrCircuitOpen = errors.New("circuit breaker is open")
	// ErrTooManyRequests is returned when half-open circuit rejects request
	ErrTooManyRequests = errors.New("too many requests")
)

// State represents circuit breaker state
type State int

const (
	// StateClosed allows all requests
	StateClosed State = iota
	// StateOpen rejects all requests
	StateOpen
	// StateHalfOpen allows limited requests to test recovery
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Config holds circuit breaker configuration
type Config struct {
	// MaxRequests is the max requests allowed in half-open state
	MaxRequests uint32
	// Interval is the cyclic period in closed state to clear successes/failures
	Interval time.Duration
	// Timeout is the period of open state before switching to half-open
	Timeout time.Duration
	// ReadyToTrip is called when circuit breaker decides whether to trip open
	// Returns true if the circuit should trip open
	ReadyToTrip func(counts Counts) bool
	// OnStateChange is called when state changes
	OnStateChange func(name string, from State, to State)
}

// Counts holds success/failure counters
type Counts struct {
	Requests             uint32
	TotalSuccesses       uint32
	TotalFailures        uint32
	ConsecutiveSuccesses uint32
	ConsecutiveFailures  uint32
}

// CircuitBreaker prevents cascading failures by failing fast
type CircuitBreaker struct {
	name          string
	maxRequests   uint32
	interval      time.Duration
	timeout       time.Duration
	readyToTrip   func(counts Counts) bool
	onStateChange func(name string, from State, to State)

	mu              sync.RWMutex
	state           State
	generation      uint64
	counts          Counts
	expiry          time.Time
	halfOpenAllowed uint32
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(name string, config Config) *CircuitBreaker {
	cb := &CircuitBreaker{
		name:          name,
		maxRequests:   config.MaxRequests,
		interval:      config.Interval,
		timeout:       config.Timeout,
		readyToTrip:   config.ReadyToTrip,
		onStateChange: config.OnStateChange,
	}

	if cb.maxRequests == 0 {
		cb.maxRequests = 1
	}

	if cb.interval == 0 {
		cb.interval = 60 * time.Second
	}

	if cb.timeout == 0 {
		cb.timeout = 60 * time.Second
	}

	if cb.readyToTrip == nil {
		// Default: trip if 5+ consecutive failures OR 50%+ failure rate with 10+ requests
		cb.readyToTrip = func(counts Counts) bool {
			return counts.ConsecutiveFailures >= 5 ||
				(counts.Requests >= 10 && float64(counts.TotalFailures)/float64(counts.Requests) >= 0.5)
		}
	}

	cb.toNewGeneration(time.Now())

	return cb
}

// Execute runs the given function if circuit breaker allows it
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() error) error {
	generation, err := cb.beforeRequest()
	if err != nil {
		return err
	}

	// Check context before executing
	select {
	case <-ctx.Done():
		cb.afterRequest(generation, false)
		return ctx.Err()
	default:
	}

	// Execute function
	err = fn()

	// Record result
	cb.afterRequest(generation, err == nil)

	return err
}

// beforeRequest checks if request should be allowed
func (cb *CircuitBreaker) beforeRequest() (uint64, error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	state, generation := cb.currentState(now)

	if state == StateOpen {
		return generation, ErrCircuitOpen
	}

	if state == StateHalfOpen {
		if cb.halfOpenAllowed >= cb.maxRequests {
			return generation, ErrTooManyRequests
		}
		cb.halfOpenAllowed++
	}

	cb.counts.Requests++
	return generation, nil
}

// afterRequest records the result of a request
func (cb *CircuitBreaker) afterRequest(generation uint64, success bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	state, currentGen := cb.currentState(now)

	// Ignore if generation has changed (circuit state changed during request)
	if generation != currentGen {
		return
	}

	if success {
		cb.onSuccess(state, now)
	} else {
		cb.onFailure(state, now)
	}
}

// onSuccess handles successful request
func (cb *CircuitBreaker) onSuccess(state State, now time.Time) {
	cb.counts.TotalSuccesses++
	cb.counts.ConsecutiveSuccesses++
	cb.counts.ConsecutiveFailures = 0

	if state == StateHalfOpen {
		// If we've reached max successful requests in half-open, close the circuit
		if cb.counts.ConsecutiveSuccesses >= cb.maxRequests {
			cb.setState(StateClosed, now)
		}
	}
}

// onFailure handles failed request
func (cb *CircuitBreaker) onFailure(state State, now time.Time) {
	cb.counts.TotalFailures++
	cb.counts.ConsecutiveFailures++
	cb.counts.ConsecutiveSuccesses = 0

	if cb.readyToTrip(cb.counts) {
		cb.setState(StateOpen, now)
	}
}

// currentState returns current state, updating if needed
func (cb *CircuitBreaker) currentState(now time.Time) (State, uint64) {
	switch cb.state {
	case StateClosed:
		if !cb.expiry.IsZero() && cb.expiry.Before(now) {
			cb.toNewGeneration(now)
		}
	case StateOpen:
		if cb.expiry.Before(now) {
			cb.setState(StateHalfOpen, now)
		}
	}

	return cb.state, cb.generation
}

// setState changes the circuit breaker state
func (cb *CircuitBreaker) setState(state State, now time.Time) {
	if cb.state == state {
		return
	}

	prev := cb.state
	cb.state = state

	cb.toNewGeneration(now)

	if cb.onStateChange != nil {
		cb.onStateChange(cb.name, prev, state)
	}
}

// toNewGeneration starts a new generation
func (cb *CircuitBreaker) toNewGeneration(now time.Time) {
	cb.generation++
	cb.counts = Counts{}
	cb.halfOpenAllowed = 0

	var zero time.Time
	switch cb.state {
	case StateClosed:
		if cb.interval == 0 {
			cb.expiry = zero
		} else {
			cb.expiry = now.Add(cb.interval)
		}
	case StateOpen:
		cb.expiry = now.Add(cb.timeout)
	default: // StateHalfOpen
		cb.expiry = zero
	}
}

// State returns current state
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return cb.state
}

// Counts returns current counts
func (cb *CircuitBreaker) Counts() Counts {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return cb.counts
}

// Name returns circuit breaker name
func (cb *CircuitBreaker) Name() string {
	return cb.name
}

// Reset resets circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.setState(StateClosed, time.Now())
}

// TwoStepCircuitBreaker allows manual success/failure reporting
type TwoStepCircuitBreaker struct {
	cb *CircuitBreaker
}

// NewTwoStepCircuitBreaker creates a two-step circuit breaker
func NewTwoStepCircuitBreaker(name string, config Config) *TwoStepCircuitBreaker {
	return &TwoStepCircuitBreaker{
		cb: NewCircuitBreaker(name, config),
	}
}

// Allow checks if request is allowed
func (tscb *TwoStepCircuitBreaker) Allow() (done func(success bool), err error) {
	generation, err := tscb.cb.beforeRequest()
	if err != nil {
		return nil, err
	}

	return func(success bool) {
		tscb.cb.afterRequest(generation, success)
	}, nil
}

// State returns current state
func (tscb *TwoStepCircuitBreaker) State() State {
	return tscb.cb.State()
}

// Counts returns current counts
func (tscb *TwoStepCircuitBreaker) Counts() Counts {
	return tscb.cb.Counts()
}

// Reset resets to closed state
func (tscb *TwoStepCircuitBreaker) Reset() {
	tscb.cb.Reset()
}
