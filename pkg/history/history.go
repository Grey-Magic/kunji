// Package history persists per-key validation records across runs so users
// can answer "what was the validity of this key over the last N runs?".
//
// Records are keyed on (provider, sha256(key)) and include the run_id so a
// single key's history can be grouped and sorted by time.
//
// On-disk format: ~/.kunji/history.jsonl — one JSON object per line.
package history

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/Grey-Magic/kunji/pkg/audit"
	"github.com/Grey-Magic/kunji/pkg/models"
)

// Record is one history entry.
type Record struct {
	RunID      string `json:"run_id"`
	Timestamp  string `json:"ts"`
	Provider   string `json:"provider"`
	KeySHA256  string `json:"key_sha256"`
	IsValid    bool   `json:"is_valid"`
	StatusCode int    `json:"status_code,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
}

// Logger writes history records. A nil receiver is a no-op.
type Logger struct {
	mu    sync.Mutex
	path  string
	runID string
	f     *os.File
	enc   *json.Encoder
}

// DefaultPath returns ~/.kunji/history.jsonl (override with KUNJI_HISTORY_FILE).
func DefaultPath() string {
	if p := os.Getenv("KUNJI_HISTORY_FILE"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".kunji", "history.jsonl")
	}
	return filepath.Join(home, ".kunji", "history.jsonl")
}

// NewRunID produces a short per-run identifier suitable for tagging records.
// Format: "<unix-nano>-<8 hex chars>" where the hex chars are a small hash
// of a per-process counter plus the timestamp, so two runs started in the
// same nanosecond don't collide even within the same process.
var runIDSalt uint64

func NewRunID() string {
	now := time.Now().UnixNano()
	runIDSalt++
	salt := make([]byte, 12)
	binary.BigEndian.PutUint64(salt[:8], uint64(now))
	binary.BigEndian.PutUint32(salt[8:], uint32(runIDSalt))
	h := sha256.Sum256(salt)
	return fmt.Sprintf("%d-%s", now, hex.EncodeToString(h[:4]))
}

// Open prepares a history logger. runID should be unique per execution and is
// stamped on every record written by this logger.
func Open(path, runID string) (*Logger, error) {
	if path == "" {
		return &Logger{}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("history: mkdir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("history: open %s: %w", path, err)
	}
	return &Logger{path: path, runID: runID, f: f, enc: json.NewEncoder(f)}, nil
}

// RunID returns the run identifier stamped on records.
func (l *Logger) RunID() string {
	if l == nil {
		return ""
	}
	return l.runID
}

// Emit writes one history record for the given validation result.
func (l *Logger) Emit(r *models.ValidationResult) error {
	if l == nil || r == nil || l.f == nil {
		return nil
	}
	rec := Record{
		RunID:      l.runID,
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
		Provider:   r.Provider,
		KeySHA256:  audit.HashKey(r.Key),
		IsValid:    r.IsValid,
		StatusCode: r.StatusCode,
		DurationMs: int64(r.ResponseTime * 1000),
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.enc.Encode(&rec)
}

// Close flushes and releases the underlying file. Idempotent.
func (l *Logger) Close() error {
	if l == nil || l.f == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	err := l.f.Close()
	l.f = nil
	return err
}

// Path returns the on-disk path ("" for the no-op logger).
func (l *Logger) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

// ReadAll loads the entire history file. Used by `kunji history`.
func ReadAll(path string) ([]Record, error) {
	if path == "" {
		return nil, nil
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	dec := json.NewDecoder(f)
	var out []Record
	for dec.More() {
		var r Record
		if err := dec.Decode(&r); err != nil {
			// Tolerate partial / corrupt files: stop at first decode error.
			break
		}
		out = append(out, r)
	}
	return out, nil
}

// ByKey groups records by their (provider, key_sha256) composite key, sorted
// newest-first within each group.
func ByKey(records []Record) map[string][]Record {
	out := make(map[string][]Record)
	for _, r := range records {
		k := r.Provider + "|" + r.KeySHA256
		out[k] = append(out[k], r)
	}
	for k := range out {
		rs := out[k]
		sort.Slice(rs, func(i, j int) bool {
			return rs[i].Timestamp > rs[j].Timestamp
		})
		out[k] = rs
	}
	return out
}

// Summary describes the latest state of one key.
type Summary struct {
	Provider    string
	KeySHA256   string
	Latest      Record
	History     []Record
	ValidStreak int
	TotalRuns   int
	ValidRuns   int
}

// Summarize produces one Summary per distinct key, including its latest
// record and a simple validity streak.
func Summarize(records []Record) []Summary {
	groups := ByKey(records)
	out := make([]Summary, 0, len(groups))
	for k, rs := range groups {
		s := Summary{
			Provider:  rs[0].Provider,
			KeySHA256: rs[0].KeySHA256,
			Latest:    rs[0],
			History:   rs,
			TotalRuns: len(rs),
		}
		for _, r := range rs {
			if r.IsValid {
				s.ValidRuns++
			}
		}
		for _, r := range rs {
			if r.IsValid {
				s.ValidStreak++
			} else {
				break
			}
		}
		_ = k
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Latest.Timestamp != out[j].Latest.Timestamp {
			return out[i].Latest.Timestamp > out[j].Latest.Timestamp
		}
		return out[i].KeySHA256 < out[j].KeySHA256
	})
	return out
}
