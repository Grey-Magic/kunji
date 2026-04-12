package client

import (
	"sync"
	"time"

	"github.com/Grey-Magic/kunji/pkg/models"
)

type cacheEntry struct {
	result    *models.ValidationResult
	expiresAt time.Time
}

type ValidationCache struct {
	entries map[string]cacheEntry
	mu      sync.RWMutex
	ttl     time.Duration
	maxSize int
	hits    int64
	misses  int64
}

func NewValidationCache(ttl time.Duration, maxSize int) *ValidationCache {
	c := &ValidationCache{
		entries: make(map[string]cacheEntry, maxSize),
		ttl:     ttl,
		maxSize: maxSize,
	}

	go c.evictLoop()
	return c
}

func (c *ValidationCache) cacheKey(provider, apiKey string) string {
	return provider + ":" + apiKey
}

func (c *ValidationCache) Get(provider, apiKey string) (*models.ValidationResult, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := c.cacheKey(provider, apiKey)
	entry, ok := c.entries[key]
	if !ok {
		c.misses++
		return nil, false
	}

	if time.Now().After(entry.expiresAt) {
		c.misses++
		return nil, false
	}

	c.hits++

	result := *entry.result
	return &result, true
}

func (c *ValidationCache) Set(provider, apiKey string, result *models.ValidationResult) {
	if result == nil {
		return
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if len(c.entries) >= c.maxSize {
		c.evictExpiredLocked()
		if len(c.entries) >= c.maxSize {
			oldestKey := ""
			oldestTime := time.Time{}
			for k, e := range c.entries {
				if oldestKey == "" || e.expiresAt.Before(oldestTime) {
					oldestKey = k
					oldestTime = e.expiresAt
				}
			}
			if oldestKey != "" {
				delete(c.entries, oldestKey)
			}
		}
	}

	key := c.cacheKey(provider, apiKey)
	c.entries[key] = cacheEntry{
		result:    result,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *ValidationCache) evictExpiredLocked() {
	now := time.Now()
	for k, e := range c.entries {
		if now.After(e.expiresAt) {
			delete(c.entries, k)
		}
	}
}

func (c *ValidationCache) evictLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		c.mu.Lock()
		c.evictExpiredLocked()
		c.mu.Unlock()
	}
}

func (c *ValidationCache) Stats() (hits, misses, size int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return int(c.hits), int(c.misses), len(c.entries)
}
