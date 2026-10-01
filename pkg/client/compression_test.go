package client

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Grey-Magic/kunji/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEncodeDecodeCacheLine_Roundtrip(t *testing.T) {
	prev := compressionEnabled
	defer func() { compressionEnabled = prev }()
	compressionEnabled = true

	e := persistentCacheEntry{
		Key:       "abc123",
		Observed:  time.Now(),
		ExpiresAt: time.Now().Add(time.Hour),
		Result:    &models.ValidationResult{Provider: "openai", Key: "sk-x", IsValid: true, StatusCode: 200},
	}

	line, err := encodeCacheLine(e)
	require.NoError(t, err)
	require.NotEmpty(t, line)
	assert.Equal(t, byte('Z'), line[0], "compressed line must start with the marker byte")

	got, err := decodeCacheLine(line[:len(line)-1])
	require.NoError(t, err)
	assert.Equal(t, e.Key, got.Key)
	assert.Equal(t, e.Result.Provider, got.Result.Provider)
	assert.Equal(t, e.Result.IsValid, got.Result.IsValid)
}

func TestEncodeCacheLine_PlainWhenCompressionDisabled(t *testing.T) {
	prev := compressionEnabled
	defer func() { compressionEnabled = prev }()
	compressionEnabled = false

	e := persistentCacheEntry{
		Key:      "k",
		Observed: time.Now(),
		Result:   &models.ValidationResult{Provider: "x", IsValid: true},
	}
	line, err := encodeCacheLine(e)
	require.NoError(t, err)
	assert.NotEqual(t, byte('Z'), line[0], "plain line must not start with the marker byte")
}

func TestPersistentCache_ReadsMixedLegacyAndCompressedLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cache.jsonl")

	// Write a legacy plain line directly using a real CacheKey so the
	// legacy line is actually retrievable.
	keyHash := CacheKey("p", "k")
	plain := `{"key":"` + keyHash + `","observed":"2026-01-01T00:00:00Z","expires_at":"2099-01-01T00:00:00Z","result":{"provider":"p","key":"k","is_valid":true,"status_code":200}}` + "\n"
	require.NoError(t, os.WriteFile(path, []byte(plain), 0o600))

	// Append a compressed line for the same key with a fresher observed time.
	c, err := NewPersistentCache(path, time.Hour)
	require.NoError(t, err)

	c.Set("p", "k", &models.ValidationResult{Provider: "p", Key: "k", IsValid: true, StatusCode: 200})

	got, ok := c.Get("p", "k")
	require.True(t, ok)
	assert.True(t, got.IsValid)
}

func TestPersistentCache_LegacyFileStillWorks(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.jsonl")

	// Build the legacy line using a real CacheKey so the lookup matches.
	keyHash := CacheKey("p", "k")
	plain := `{"key":"` + keyHash + `","observed":"2026-01-01T00:00:00Z","expires_at":"2099-01-01T00:00:00Z","result":{"provider":"p","key":"k","is_valid":false,"status_code":401}}` + "\n"
	require.NoError(t, os.WriteFile(path, []byte(plain), 0o600))

	c, err := NewPersistentCache(path, time.Hour)
	require.NoError(t, err)
	got, ok := c.Get("p", "k")
	require.True(t, ok)
	assert.False(t, got.IsValid)
}

func TestPersistentCache_CompressedRoundTripOnDisk(t *testing.T) {
	// Force compression on in case a previous test left it disabled.
	SetCompressionEnabled(true)
	t.Cleanup(func() { SetCompressionEnabled(true) })

	dir := t.TempDir()
	path := filepath.Join(dir, "compressed.jsonl")

	c, err := NewPersistentCache(path, time.Hour)
	require.NoError(t, err)

	c.Set("openai", "sk-aaa", &models.ValidationResult{Provider: "openai", Key: "sk-aaa", IsValid: true, StatusCode: 200})

	data, err := os.ReadFile(path)
	require.NoError(t, err)

	lines := bytes.Split(bytes.TrimRight(data, "\n"), []byte("\n"))
	require.NotEmpty(t, lines)
	assert.Equal(t, byte('Z'), lines[0][0], "the on-disk file must use the compressed marker")

	c2, err := NewPersistentCache(path, time.Hour)
	require.NoError(t, err)
	got, ok := c2.Get("openai", "sk-aaa")
	require.True(t, ok)
	assert.True(t, got.IsValid)
}

func TestPersistentCache_CompressionReducesSize(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping size test in short mode")
	}

	dir := t.TempDir()
	plainPath := filepath.Join(dir, "plain.jsonl")
	compPath := filepath.Join(dir, "comp.jsonl")

	plain, err := NewPersistentCache(plainPath, time.Hour)
	require.NoError(t, err)
	comp, err := NewPersistentCache(compPath, time.Hour)
	require.NoError(t, err)

	prev := compressionEnabled
	compressionEnabled = false
	defer func() { compressionEnabled = prev }()

	const N = 200
	for i := 0; i < N; i++ {
		res := &models.ValidationResult{
			Provider:    "openai",
			Key:         "sk-abcdef1234567890abcdef1234567890",
			IsValid:     true,
			StatusCode:  200,
			AccountName: "Test User With Somewhat Repetitive Boilerplate Data",
			Email:       "test@example.com",
			Extra: map[string]interface{}{
				"region": "us-east-1",
				"tier":   "premium",
				"flags":  []interface{}{"a", "b", "c", "d"},
			},
		}
		plain.Set("openai", "sk-aaa", res)
		_ = plain
		compressionEnabled = true
		comp.Set("openai", "sk-aaa", res)
		compressionEnabled = false
	}

	pSize, _ := os.Stat(plainPath)
	cSize, _ := os.Stat(compPath)

	// Compression should make the file at least somewhat smaller.
	// We don't enforce a hard ratio because JSON on small entries may not
	// compress well, but for repeated boilerplate it should clearly help.
	t.Logf("plain=%d compressed=%d", pSize.Size(), cSize.Size())
	assert.Less(t, cSize.Size(), pSize.Size(),
		"compressed cache should be smaller than the plain cache")
}

// helper that ensures the imports stay referenced when tests change.
var (
	_ = gzip.NewWriter
	_ = base64.StdEncoding
	_ = time.Second
)
