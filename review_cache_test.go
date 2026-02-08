package main

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func TestReviewCacheGetSet(t *testing.T) {
	cache := NewReviewCache()

	review := "Test review content"
	cache.Set("/path/to/wt1", 123, review)

	retrieved := cache.Get("/path/to/wt1", 123)
	if retrieved == nil {
		t.Errorf("Expected to retrieve review, got nil")
	}
	if *retrieved != review {
		t.Errorf("Expected review %q, got %q", review, *retrieved)
	}
}

func TestReviewCacheGetNonexistent(t *testing.T) {
	cache := NewReviewCache()

	retrieved := cache.Get("/nonexistent", 999)
	if retrieved != nil {
		t.Errorf("Expected nil for nonexistent review, got %v", retrieved)
	}
}

func TestReviewCacheRemove(t *testing.T) {
	cache := NewReviewCache()

	cache.Set("/path/to/wt1", 123, "review1")
	cache.Set("/path/to/wt1", 456, "review2")

	// Remove one PR review
	cache.Remove("/path/to/wt1", 123)

	if retrieved := cache.Get("/path/to/wt1", 123); retrieved != nil {
		t.Errorf("Expected removed review to be nil, got %v", retrieved)
	}

	// Other PR review should still exist
	if retrieved := cache.Get("/path/to/wt1", 456); retrieved == nil {
		t.Errorf("Expected other review to still exist")
	}
}

func TestReviewCacheRemoveWorktree(t *testing.T) {
	cache := NewReviewCache()

	cache.Set("/path/to/wt1", 123, "review1")
	cache.Set("/path/to/wt1", 456, "review2")
	cache.Set("/path/to/wt2", 789, "review3")

	cache.RemoveWorktree("/path/to/wt1")

	// All wt1 reviews should be gone
	if retrieved := cache.Get("/path/to/wt1", 123); retrieved != nil {
		t.Errorf("Expected removed worktree reviews to be nil")
	}
	if retrieved := cache.Get("/path/to/wt1", 456); retrieved != nil {
		t.Errorf("Expected removed worktree reviews to be nil")
	}

	// wt2 reviews should still exist
	if retrieved := cache.Get("/path/to/wt2", 789); retrieved == nil {
		t.Errorf("Expected other worktree review to still exist")
	}
}

func TestReviewCacheConcurrentReads(t *testing.T) {
	cache := NewReviewCache()

	cache.Set("/path/wt", 1, "review1")

	const numGoroutines = 100
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			retrieved := cache.Get("/path/wt", 1)
			if retrieved == nil {
				t.Errorf("Expected review to be available")
			}
		}()
	}

	wg.Wait()
}

func TestReviewCacheConcurrentReadWrite(t *testing.T) {
	cache := NewReviewCache()

	var readCount int32
	var writeCount int32
	var wg sync.WaitGroup

	// Writer goroutine
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			review := fmt.Sprintf("review_%d", i)
			cache.Set("/path/wt", i%10, review)
			atomic.AddInt32(&writeCount, 1)
		}
	}()

	// Reader goroutines
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				// Try to read any PR
				cache.Get("/path/wt", j%10)
				atomic.AddInt32(&readCount, 1)
			}
		}()
	}

	wg.Wait()

	if atomic.LoadInt32(&writeCount) != 100 {
		t.Errorf("Expected 100 writes, got %d", writeCount)
	}
	if atomic.LoadInt32(&readCount) != 500 {
		t.Errorf("Expected 500 reads, got %d", readCount)
	}
}

func TestReviewCacheGetAllForWorktree(t *testing.T) {
	cache := NewReviewCache()

	cache.Set("/wt1", 1, "review1")
	cache.Set("/wt1", 2, "review2")
	cache.Set("/wt2", 3, "review3")

	all := cache.GetAllForWorktree("/wt1")
	if len(all) != 2 {
		t.Errorf("Expected 2 reviews for wt1, got %d", len(all))
	}

	if val, ok := all[1]; !ok || val != "review1" {
		t.Errorf("Expected review1 in wt1 results")
	}
	if val, ok := all[2]; !ok || val != "review2" {
		t.Errorf("Expected review2 in wt1 results")
	}

	// Modifying returned map should not affect cache
	all[1] = "modified"
	if retrieved := cache.Get("/wt1", 1); retrieved != nil && *retrieved != "review1" {
		t.Errorf("Cache was modified by external mutation")
	}
}

func TestReviewCacheClear(t *testing.T) {
	cache := NewReviewCache()

	cache.Set("/wt1", 1, "review1")
	cache.Set("/wt2", 2, "review2")

	cache.Clear()

	if retrieved := cache.Get("/wt1", 1); retrieved != nil {
		t.Errorf("Expected cleared cache to be empty")
	}
	if retrieved := cache.Get("/wt2", 2); retrieved != nil {
		t.Errorf("Expected cleared cache to be empty")
	}
}
