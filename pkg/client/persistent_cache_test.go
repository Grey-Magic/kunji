package client

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Grey-Magic/kunji/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPersistentCache_SetAndGet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.jsonl")
	c, err := NewPersistentCache(path, 5*time.Minute)
	require.NoError(t, err)

	res := &models.ValidationResult{Provider: "openai", Key: "sk-abc", IsValid: true}
	c.Set("openai", "sk-abc", res)

	got, ok := c.Get("openai", "sk-abc")
	require.True(t, ok)
	assert.Equal(t, "openai", got.Provider)
	assert.Equal(t, "sk-abc", got.Key)
	assert.True(t, got.IsValid)
}

func TestPersistentCache_MissAfterTTL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.jsonl")
	c, err := NewPersistentCache(path, 50*time.Millisecond)
	require.NoError(t, err)

	c.Set("p", "k", &models.ValidationResult{Provider: "p", Key: "k", IsValid: true})

	time.Sleep(80 * time.Millisecond)
	_, ok := c.Get("p", "k")
	assert.False(t, ok, "entry should be expired")
}

func TestPersistentCache_PersistsAcrossInstances(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.jsonl")

	c1, err := NewPersistentCache(path, time.Minute)
	require.NoError(t, err)
	c1.Set("openai", "sk-abc", &models.ValidationResult{Provider: "openai", Key: "sk-abc", IsValid: true})
	c1.Set("stripe", "sk_live_xyz", &models.ValidationResult{Provider: "stripe", Key: "sk_live_xyz", IsValid: false})

	// New instance reads the same file.
	c2, err := NewPersistentCache(path, time.Minute)
	require.NoError(t, err)

	got1, ok := c2.Get("openai", "sk-abc")
	require.True(t, ok)
	assert.True(t, got1.IsValid)

	got2, ok := c2.Get("stripe", "sk_live_xyz")
	require.True(t, ok)
	assert.False(t, got2.IsValid)
}

func TestPersistentCache_DisabledNeverStores(t *testing.T) {
	c := NewDisabledPersistentCache()
	c.Set("p", "k", &models.ValidationResult{Provider: "p", Key: "k", IsValid: true})

	_, ok := c.Get("p", "k")
	assert.False(t, ok)
	assert.False(t, c.Enabled())
}

func TestPersistentCache_LaterEntryWins(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.jsonl")
	c, err := NewPersistentCache(path, time.Minute)
	require.NoError(t, err)

	c.Set("p", "k", &models.ValidationResult{Provider: "p", Key: "k", IsValid: true})
	time.Sleep(5 * time.Millisecond)
	c.Set("p", "k", &models.ValidationResult{Provider: "p", Key: "k", IsValid: false})

	got, ok := c.Get("p", "k")
	require.True(t, ok)
	assert.False(t, got.IsValid, "later Set should override earlier entry")
}

func TestPersistentCache_KeysAreHashedOnDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.jsonl")
	c, err := NewPersistentCache(path, time.Minute)
	require.NoError(t, err)

	c.Set("p", "supersecretvalue", &models.ValidationResult{IsValid: true})

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "supersecretvalue",
		"raw key must never appear in the cache file")
}

func TestLayeredCache_FallsThroughToDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.jsonl")
	disk, err := NewPersistentCache(path, time.Minute)
	require.NoError(t, err)

	// Layered cache with no in-memory layer: reads must hit disk.
	l := NewLayeredCache(nil, disk)
	l.Set("p", "k", &models.ValidationResult{Provider: "p", Key: "k", IsValid: true})

	r, ok := l.Get("p", "k")
	require.True(t, ok)
	assert.True(t, r.IsValid)
}

func TestLayeredCache_PromotesDiskHitToMem(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.jsonl")
	disk, err := NewPersistentCache(path, time.Minute)
	require.NoError(t, err)
	mem := NewValidationCache(time.Minute, 100)

	// Pre-populate only the disk layer.
	disk.Set("p", "k", &models.ValidationResult{Provider: "p", Key: "k", IsValid: true})

	l := NewLayeredCache(mem, disk)
	_, ok := l.Get("p", "k")
	require.True(t, ok)

	// Now the in-memory layer should have it.
	if cached, ok := mem.Get("p", "k"); !ok || !cached.IsValid {
		t.Fatalf("expected in-memory promotion after disk hit")
	}
}

func TestLayeredCache_NilSafe(t *testing.T) {
	var l *LayeredCache
	_, ok := l.Get("p", "k")
	assert.False(t, ok)
	// Set must not panic.
	l.Set("p", "k", &models.ValidationResult{Provider: "p", Key: "k", IsValid: true})
}
