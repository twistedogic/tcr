package main

import (
	"sync"
	"time"
)

// RateLimiter enforces a maximum number of requests per second using a token bucket algorithm.
// It distributes tokens evenly across the second to prevent burst-then-wait patterns.
type RateLimiter struct {
	maxPerSecond               int
	mu                         sync.Mutex
	lastRequestTime            time.Time
	minIntervalBetweenRequests time.Duration
}

// NewRateLimiter creates a new rate limiter with the specified max requests per second.
func NewRateLimiter(maxRequestsPerSecond int) *RateLimiter {
	return &RateLimiter{
		maxPerSecond:               maxRequestsPerSecond,
		minIntervalBetweenRequests: time.Second / time.Duration(maxRequestsPerSecond),
		lastRequestTime:            time.Now().Add(-time.Second), // Start with a full bucket
	}
}

// Wait blocks until a token is available, then consumes it.
// This ensures requests are throttled to the maximum rate by enforcing
// minimum time between consecutive requests.
func (rl *RateLimiter) Wait() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	timeSinceLastRequest := now.Sub(rl.lastRequestTime)

	if timeSinceLastRequest < rl.minIntervalBetweenRequests {
		// Need to wait
		waitTime := rl.minIntervalBetweenRequests - timeSinceLastRequest
		rl.mu.Unlock()
		time.Sleep(waitTime)
		rl.mu.Lock()
	}

	rl.lastRequestTime = time.Now()
}

// Stop is a no-op for this rate limiter implementation.
func (rl *RateLimiter) Stop() {
	// No-op: this implementation doesn't use background goroutines
}
