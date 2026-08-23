package client

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/Grey-Magic/kunji/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersistentCache_SetWithTTL_PerCallOverride(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.jsonl")

	pc, err := NewPersistentCache(path, 5*time.Minute)
	require.NoError(t, err)

	res := &models.ValidationResult{Provider: "x", Key: "k", IsValid: true}

	t.Run("zero ttl falls back to default", func(t *testing.T) {
		pc.SetWithTTL("x", "k1", res, 0)
		got, ok := pc.Get("x", "k1")
		assert.True(t, ok)
		assert.True(t, got.IsValid)
	})

	t.Run("explicit ttl honored", func(t *testing.T) {
		pc.SetWithTTL("x", "k2", res, 24*time.Hour)
		// Force the in-memory entry to have an ExpiresAt that's far in the future.
		got, ok := pc.Get("x", "k2")
		assert.True(t, ok)
		assert.True(t, got.IsValid)
		// Disk entry should also reflect the override.
		pc.mu.RLock()
		e := pc.entries[CacheKey("x", "k2")]
		pc.mu.RUnlock()
		assert.True(t, time.Until(e.ExpiresAt) > 1*time.Hour)
	})

	t.Run("very short ttl expires the entry on next Get", func(t *testing.T) {
		pc.SetWithTTL("x", "k3", res, 1*time.Millisecond)
		time.Sleep(50 * time.Millisecond)
		_, ok := pc.Get("x", "k3")
		assert.False(t, ok, "expired entry should be a miss")
	})
}

func TestLayeredCache_SetWithTTL_ForwardsToBothLayers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.jsonl")

	mem := NewValidationCache(5*time.Minute, 100)
	disk, err := NewPersistentCache(path, 5*time.Minute)
	require.NoError(t, err)

	lc := NewLayeredCache(mem, disk)
	res := &models.ValidationResult{Provider: "x", Key: "k", IsValid: true}

	lc.SetWithTTL("x", "k", res, 24*time.Hour)

	// Both layers must serve the entry.
	fromMem, ok := mem.Get("x", "k")
	assert.True(t, ok)
	assert.True(t, fromMem.IsValid)
	fromDisk, ok := disk.Get("x", "k")
	assert.True(t, ok)
	assert.True(t, fromDisk.IsValid)
}

func TestValidationCache_SetWithTTL_OverrideBeatsDefault(t *testing.T) {
	c := NewValidationCache(1*time.Second, 100)
	res := &models.ValidationResult{Provider: "x", Key: "k", IsValid: true}

	c.SetWithTTL("x", "k-long", res, 1*time.Hour)
	c.SetWithTTL("x", "k-short", res, 1*time.Millisecond)

	time.Sleep(50 * time.Millisecond)

	if _, ok := c.Get("x", "k-short"); ok {
		t.Fatal("short-ttl entry should be expired")
	}
	if _, ok := c.Get("x", "k-long"); !ok {
		t.Fatal("long-ttl entry should still be live")
	}
}
