package client

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Grey-Magic/kunji/pkg/models"
)

// compressMarker is the byte that prefixes every compressed cache line.
// Lines without this marker are treated as plain JSON (legacy format).
const compressMarker byte = 'Z'

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
		entry, err := decodeCacheLine(line)
		if err != nil {
			// Skip malformed lines; they may belong to an older schema.
			continue
		}
		// Discard expired entries on load so the in-memory map stays small.
		if !entry.ExpiresAt.IsZero() && now.After(entry.ExpiresAt) {
			continue
		}
		// Latest entry for a key wins (later appends override earlier ones).
		if existing, ok := c.entries[entry.Key]; !ok || entry.Observed.After(existing.Observed) {
			c.entries[entry.Key] = entry
		}
	}
	return scanner.Err()
}

// decodeCacheLine handles both legacy plain-JSON lines and new gzip+base64
// compressed lines (prefixed with compressMarker).
func decodeCacheLine(line []byte) (persistentCacheEntry, error) {
	var rawJSON []byte

	if len(line) > 0 && line[0] == compressMarker {
		raw, err := base64.StdEncoding.DecodeString(string(line[1:]))
		if err != nil {
			return persistentCacheEntry{}, err
		}
		gz, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return persistentCacheEntry{}, err
		}
		defer gz.Close()
		rawJSON, err = io.ReadAll(gz)
		if err != nil {
			return persistentCacheEntry{}, err
		}
	} else {
		rawJSON = line
	}

	var e persistentCacheEntry
	if err := json.Unmarshal(rawJSON, &e); err != nil {
		return persistentCacheEntry{}, err
	}
	return e, nil
}

// encodeCacheLine returns the on-disk form of one entry. When compression is
// enabled the result is "Z<base64(gzip(json))>"; otherwise it is the JSON
// bytes themselves.
func encodeCacheLine(e persistentCacheEntry) ([]byte, error) {
	raw, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	if !compressionEnabled {
		out := make([]byte, 0, len(raw)+1)
		out = append(out, raw...)
		out = append(out, '\n')
		return out, nil
	}

	var buf bytes.Buffer
	buf.WriteByte(compressMarker)

	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(raw); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}

	encoded := base64.StdEncoding.EncodeToString(buf.Bytes()[1:])
	out := make([]byte, 0, len(encoded)+2)
	out = append(out, compressMarker)
	out = append(out, []byte(encoded)...)
	out = append(out, '\n')
	return out, nil
}

// compressionEnabled is the package-wide default. Toggle at startup via
// SetCompressionEnabled. Off by default to keep legacy caches readable as-is;
// new writes always produce compressed entries when this is true.
var compressionEnabled = false

// SetCompressionEnabled flips whether appendEntry produces compressed lines.
// Existing plain lines remain valid; load handles both formats transparently.
func SetCompressionEnabled(on bool) { compressionEnabled = on }

// CompressionEnabled reports the current default.
func CompressionEnabled() bool { return compressionEnabled }

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

// Set records a fresh positive result with the cache's default TTL.
func (c *PersistentCache) Set(provider, apiKey string, result *models.ValidationResult) {
	c.SetWithTTL(provider, apiKey, result, c.ttl)
}

// SetWithTTL records a fresh positive result with a per-call TTL override.
// ttl <= 0 falls back to the cache's default. Validators use this to honor
// per-provider cache_ttl_seconds declared in their YAML schema.
func (c *PersistentCache) SetWithTTL(provider, apiKey string, result *models.ValidationResult, ttl time.Duration) {
	if c.disabled || result == nil {
		return
	}
	if ttl <= 0 {
		ttl = c.ttl
	}
	key := CacheKey(provider, apiKey)
	now := time.Now()
	e := persistentCacheEntry{
		Key:       key,
		Observed:  now,
		ExpiresAt: now.Add(ttl),
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
	b, err := encodeCacheLine(e)
	if err != nil {
		return err
	}
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

// Set writes through to both layers with the cache's default TTL.
func (l *LayeredCache) Set(provider, apiKey string, result *models.ValidationResult) {
	l.SetWithTTL(provider, apiKey, result, 0)
}

// SetWithTTL writes through to both layers with a per-call TTL override.
// ttl <= 0 means "use each layer's default".
func (l *LayeredCache) SetWithTTL(provider, apiKey string, result *models.ValidationResult, ttl time.Duration) {
	if l == nil || result == nil {
		return
	}
	if l.mem != nil {
		l.mem.SetWithTTL(provider, apiKey, result, ttl)
	}
	if l.disk != nil {
		l.disk.SetWithTTL(provider, apiKey, result, ttl)
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
	SetWithTTL(provider, apiKey string, result *models.ValidationResult, ttl time.Duration)
}

// Compile-time checks.
var (
	_ ResultCache = (*ValidationCache)(nil)
	_ ResultCache = (*LayeredCache)(nil)
)
