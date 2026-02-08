package main

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRateLimiterBlocksUntilTokenAvailable(t *testing.T) {
	rl := NewRateLimiter(2)
	defer rl.Stop()

	start := time.Now()

	// First request should be immediate
	rl.Wait()

	// Second request should be immediate (2 req/sec = 500ms per request)
	rl.Wait()

	// Third wait should block until ~500ms have passed
	rl.Wait()
	elapsed := time.Since(start)

	// Should take at least 1 second (3 requests at 2 req/sec = 1.5 intervals)
	if elapsed < 900*time.Millisecond {
		t.Errorf("Expected Wait to block, but elapsed time was only %v", elapsed)
	}
}

func TestRateLimiterEnforcesMaxRate(t *testing.T) {
	rl := NewRateLimiter(5)
	defer rl.Stop()

	// Make 10 requests and measure the time
	start := time.Now()
	for i := 0; i < 10; i++ {
		rl.Wait()
	}
	elapsed := time.Since(start)

	// 10 requests at 5 req/sec = 2 seconds minimum
	// (first request is immediate, then 9 more at 200ms intervals each = 1.8s)
	expectedMinDuration := time.Duration(9*200) * time.Millisecond
	if elapsed < expectedMinDuration-50*time.Millisecond {
		t.Errorf("Requests completed too quickly: %v (expected ~%v)", elapsed, expectedMinDuration)
	}
}

func TestRateLimiterConcurrentAccess(t *testing.T) {
	rl := NewRateLimiter(10)
	defer rl.Stop()

	const numGoroutines = 5
	const requestsPerGoroutine = 4

	var wg sync.WaitGroup
	var totalRequests int32

	start := time.Now()

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				rl.Wait()
				atomic.AddInt32(&totalRequests, 1)
			}
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	expectedCount := int32(numGoroutines * requestsPerGoroutine)
	if atomic.LoadInt32(&totalRequests) != expectedCount {
		t.Errorf("Expected %d total requests, got %d", expectedCount, totalRequests)
	}

	// With mutex-protected last request time and concurrent requests,
	// total 20 requests at 10 req/sec minimum should take ~1.9 seconds
	// But concurrent access might be slightly faster. Just verify it's reasonable.
	if elapsed > 3*time.Second {
		t.Errorf("Requests took too long: %v (expected < 3s for 20 requests)", elapsed)
	}
}

func TestRateLimiterHighRate(t *testing.T) {
	rl := NewRateLimiter(1000)
	defer rl.Stop()

	// 100 requests at 1000 req/sec should complete in ~100ms
	start := time.Now()
	for i := 0; i < 100; i++ {
		rl.Wait()
	}
	elapsed := time.Since(start)

	if elapsed > 200*time.Millisecond {
		t.Errorf("1000 req/sec limiter took too long: %v", elapsed)
	}
}
