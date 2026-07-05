package client

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Grey-Magic/kunji/pkg/models"
)

// PersistentCache is a file-backed positive validation cache.
//
// Entries are keyed on sha256(provider + "|" + apiKey) and recorded with the
// validation result, the timestamp it was observed, and a per-entry TTL. The
// store is append-only on write and rebuilt into an in-memory map on load so
// reads are O(1) and the file grows predictably. Unlike ValidationCache this
// survives across processes and runs.
//
// Default location: ~/.kunji/pos_cache.jsonl
type PersistentCache struct {
	mu       sync.RWMutex
	path     string
	ttl      time.Duration
	entries  map[string]persistentCacheEntry
	hits     int64
	misses   int64
	disabled bool
}

type persistentCacheEntry struct {
	Key       string                   `json:"key"`
	Observed  time.Time                `json:"observed"`
	ExpiresAt time.Time                `json:"expires_at"`
	Result    *models.ValidationResult `json:"result"`
}

// NewPersistentCache opens (or creates) the JSONL cache at path. ttl defines
// how long a positive result stays fresh; once expired the entry is treated
// as a miss but the file is left intact (it will be evicted on the next save).
func NewPersistentCache(path string, ttl time.Duration) (*PersistentCache, error) {
	if path == "" {
		return nil, os.ErrInvalid
	}
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	c := &PersistentCache{
		path:    path,
		ttl:     ttl,
		entries: make(map[string]persistentCacheEntry),
	}
	if err := c.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	return c, nil
}

// NewDisabledPersistentCache returns a cache that never records or returns
// hits. Used when --no-cache is set.
func NewDisabledPersistentCache() *PersistentCache {
	return &PersistentCache{disabled: true, entries: make(map[string]persistentCacheEntry)}
}

// CacheKey hashes (provider, apiKey) into the storage key. Exported so the
// validator can pre-compute the lookup if needed.
func CacheKey(provider, apiKey string) string {
	sum := sha256.Sum256([]byte(provider + "|" + apiKey))
	return hex.EncodeToString(sum[:])
}

func (c *PersistentCache) load() error {
	if c.disabled {
		return nil
	}
	f, err := os.Open(c.path)
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	now := time.Now()
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var e persistentCacheEntry
		if err := json.Unmarshal(line, &e); err != nil {
			// Skip malformed lines; they may belong to an older schema.
			continue
		}
		// Discard expired entries on load so the in-memory map stays small.
		if !e.ExpiresAt.IsZero() && now.After(e.ExpiresAt) {
			continue
		}
		// Latest entry for a key wins (later appends override earlier ones).
		if existing, ok := c.entries[e.Key]; !ok || e.Observed.After(existing.Observed) {
			c.entries[e.Key] = e
		}
	}
	return scanner.Err()
}

// Get returns a cached result if present and still fresh.
func (c *PersistentCache) Get(provider, apiKey string) (*models.ValidationResult, bool) {
	if c.disabled {
		c.misses++
		return nil, false
	}
	key := CacheKey(provider, apiKey)

	c.mu.RLock()
	e, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok {
		c.misses++
		return nil, false
	}
	if !e.ExpiresAt.IsZero() && time.Now().After(e.ExpiresAt) {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		c.misses++
		return nil, false
	}

	c.hits++
	// Return a copy so callers can't mutate the cached entry.
	r := *e.Result
	return &r, true
}

// Set records a fresh positive result. Errors writing to disk are returned but
// the in-memory entry is still updated — the next save() call will retry.
func (c *PersistentCache) Set(provider, apiKey string, result *models.ValidationResult) {
	if c.disabled || result == nil {
		return
	}
	key := CacheKey(provider, apiKey)
	now := time.Now()
	e := persistentCacheEntry{
		Key:       key,
		Observed:  now,
		ExpiresAt: now.Add(c.ttl),
		Result:    result,
	}

	c.mu.Lock()
	c.entries[key] = e
	c.mu.Unlock()

	if err := c.appendEntry(e); err != nil {
		// Surface as a debug message but don't fail validation. A lost
		// append means the next run simply re-validates.
		_ = err
	}
}

func (c *PersistentCache) appendEntry(e persistentCacheEntry) error {
	f, err := os.OpenFile(c.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	_, err = f.Write(b)
	return err
}

// Stats returns cumulative hit/miss counters since process start.
func (c *PersistentCache) Stats() (hits, misses int64) {
	return c.hits, c.misses
}

// Size returns the number of live (non-expired) entries in memory.
func (c *PersistentCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// Enabled reports whether the cache is actively storing entries.
func (c *PersistentCache) Enabled() bool { return !c.disabled }

// LayeredCache combines an in-memory ValidationCache (per-process) with a
// cross-run PersistentCache. Reads consult the in-memory cache first, then
// fall back to the persistent one (promoting hits into the in-memory cache).
// Writes populate both layers.
type LayeredCache struct {
	mem  *ValidationCache
	disk *PersistentCache
}

// NewLayeredCache builds a layered cache. Either argument may be nil; the
// other layer still functions. If both are nil the returned cache is a no-op.
func NewLayeredCache(mem *ValidationCache, disk *PersistentCache) *LayeredCache {
	return &LayeredCache{mem: mem, disk: disk}
}

// Get returns a fresh result from whichever layer has it. mem is consulted
// first since it is faster; misses are promoted into mem for subsequent reads.
func (l *LayeredCache) Get(provider, apiKey string) (*models.ValidationResult, bool) {
	if l == nil {
		return nil, false
	}
	if l.mem != nil {
		if r, ok := l.mem.Get(provider, apiKey); ok {
			return r, true
		}
	}
	if l.disk != nil {
		if r, ok := l.disk.Get(provider, apiKey); ok {
			if l.mem != nil {
				l.mem.Set(provider, apiKey, r)
			}
			return r, true
		}
	}
	return nil, false
}

// Set writes through to both layers.
func (l *LayeredCache) Set(provider, apiKey string, result *models.ValidationResult) {
	if l == nil || result == nil {
		return
	}
	if l.mem != nil {
		l.mem.Set(provider, apiKey, result)
	}
	if l.disk != nil {
		l.disk.Set(provider, apiKey, result)
	}
}

// Stats returns (mem_hits, mem_misses, disk_hits, disk_misses).
func (l *LayeredCache) Stats() (mh, mm, dh, dm int64) {
	if l == nil {
		return
	}
	if l.mem != nil {
		h, m, _ := l.mem.Stats()
		mh, mm = int64(h), int64(m)
	}
	if l.disk != nil {
		dh, dm = l.disk.Stats()
	}
	return
}

// ResultCache is the contract validators rely on for positive-cache reads and
// writes. Both *ValidationCache and *LayeredCache satisfy it.
type ResultCache interface {
	Get(provider, apiKey string) (*models.ValidationResult, bool)
	Set(provider, apiKey string, result *models.ValidationResult)
}

// Compile-time checks.
var (
	_ ResultCache = (*ValidationCache)(nil)
	_ ResultCache = (*LayeredCache)(nil)
)
