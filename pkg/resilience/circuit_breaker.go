package resilience

import (
	"errors"
	"sync"
	"time"
)

// RateLimitError represents a provider rate limit response.
type RateLimitError struct {
	Provider string
	Message  string
}

func (e RateLimitError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return "rate limit"
}

// IsRateLimit returns true when the error is a RateLimitError.
func IsRateLimit(err error) bool {
	var rl RateLimitError
	return errors.As(err, &rl)
}

// CircuitBreaker blocks requests after repeated rate limit failures.
type CircuitBreaker struct {
	mu        sync.Mutex
	failures  int
	threshold int
	openUntil time.Time
	cooldown  time.Duration
}

func NewCircuitBreaker(threshold int, cooldown time.Duration) *CircuitBreaker {
	if threshold <= 0 {
		threshold = 3
	}
	if cooldown <= 0 {
		cooldown = 30 * time.Second
	}
	return &CircuitBreaker{threshold: threshold, cooldown: cooldown}
}

func (c *CircuitBreaker) Allow() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return !time.Now().Before(c.openUntil)
}

func (c *CircuitBreaker) OnSuccess() {
	c.mu.Lock()
	c.failures = 0
	c.openUntil = time.Time{}
	c.mu.Unlock()
}

func (c *CircuitBreaker) OnError(err error) {
	if !IsRateLimit(err) {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.failures++
	if c.failures >= c.threshold {
		c.openUntil = time.Now().Add(c.cooldown)
	}
}
