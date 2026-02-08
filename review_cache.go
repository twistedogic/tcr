package main

import (
	"sync"
)

// ReviewCache is a thread-safe in-memory cache that stores formatted review strings.
// It is indexed by worktree path and PR number for O(1) lookup and update.
type ReviewCache struct {
	mu      sync.RWMutex
	reviews map[string]map[int]string // [worktreePath][prNumber]formattedReviewString
}

// NewReviewCache creates and returns a new empty review cache.
func NewReviewCache() *ReviewCache {
	return &ReviewCache{
		reviews: make(map[string]map[int]string),
	}
}

// Get retrieves a cached review by worktree path and PR number.
// Returns nil if the review is not cached.
func (rc *ReviewCache) Get(worktreePath string, prNumber int) *string {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	if wtCache, ok := rc.reviews[worktreePath]; ok {
		if review, ok := wtCache[prNumber]; ok {
			return &review
		}
	}
	return nil
}

// Set stores a formatted review in the cache.
// If a review already exists for this worktree/PR, it is replaced.
func (rc *ReviewCache) Set(worktreePath string, prNumber int, review string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if _, ok := rc.reviews[worktreePath]; !ok {
		rc.reviews[worktreePath] = make(map[int]string)
	}
	rc.reviews[worktreePath][prNumber] = review
}

// Remove deletes a cached review for a specific worktree and PR number.
// Does nothing if the review doesn't exist.
func (rc *ReviewCache) Remove(worktreePath string, prNumber int) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	if wtCache, ok := rc.reviews[worktreePath]; ok {
		delete(wtCache, prNumber)
		// Clean up empty worktree maps
		if len(wtCache) == 0 {
			delete(rc.reviews, worktreePath)
		}
	}
}

// RemoveWorktree removes all cached reviews for a given worktree.
// Does nothing if the worktree doesn't exist in the cache.
func (rc *ReviewCache) RemoveWorktree(worktreePath string) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	delete(rc.reviews, worktreePath)
}

// GetAllForWorktree returns all cached reviews for a worktree as a map copy.
// Returns an empty map if the worktree has no cached reviews.
func (rc *ReviewCache) GetAllForWorktree(worktreePath string) map[int]string {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	if wtCache, ok := rc.reviews[worktreePath]; ok {
		// Return a copy to avoid external mutation
		result := make(map[int]string)
		for prNum, review := range wtCache {
			result[prNum] = review
		}
		return result
	}
	return make(map[int]string)
}

// Clear removes all cached reviews.
func (rc *ReviewCache) Clear() {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	rc.reviews = make(map[string]map[int]string)
}
