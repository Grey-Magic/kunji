// Package audit appends a structured record of every validation to a JSONL
// audit log. Records contain only the SHA-256 hash of the key, never the raw
// secret, so the audit log is safe to ship alongside other operational logs.
//
// Each line is a JSON object:
//
//	{"ts":"2026-...","provider":"openai","key_sha256":"...","is_valid":true,
//	 "status_code":200,"error_code":"","duration_ms":123}
//
// The file is opened once per write and held until Close(); concurrent Emit
// calls are serialized through an internal mutex.
package audit

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/Grey-Magic/kunji/pkg/models"
)

// Record is one line in the audit log.
type Record struct {
	Timestamp  string `json:"ts"`
	Provider   string `json:"provider"`
	KeySHA256  string `json:"key_sha256"`
	IsValid    bool   `json:"is_valid"`
	StatusCode int    `json:"status_code,omitempty"`
	ErrorCode  string `json:"error_code,omitempty"`
	DurationMs int64  `json:"duration_ms,omitempty"`
}

// Logger is the audit writer. A nil Logger is a valid no-op (Emit returns
// nil and Close returns nil) so callers don't need to nil-check.
type Logger struct {
	mu   sync.Mutex
	path string
	f    *os.File
	enc  *json.Encoder

	// batched controls whether writes flush every line (false) or accumulate
	// in the encoder's buffer (true). Today every Emit flushes; left as a
	// knob for future tuning.
	batched bool
}

// DefaultPath returns the conventional audit log location:
// ~/.kunji/audit.jsonl (override with KUNJI_AUDIT_FILE).
func DefaultPath() string {
	if p := os.Getenv("KUNJI_AUDIT_FILE"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".kunji", "audit.jsonl")
	}
	return filepath.Join(home, ".kunji", "audit.jsonl")
}

// Open creates or appends to the audit log at path. Missing parent dirs are
// created with 0700 perms. If path == "", a no-op Logger is returned.
func Open(path string) (*Logger, error) {
	if path == "" {
		return &Logger{}, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("audit: mkdir: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("audit: open %s: %w", path, err)
	}
	return &Logger{path: path, f: f, enc: json.NewEncoder(f)}, nil
}

// HashKey returns the SHA-256 hex digest of the key. Exposed so callers
// (and tests) can reproduce the audit hash independently.
func HashKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// Emit writes one audit record. Safe for concurrent use. A nil receiver is a
// no-op so callers don't need to guard.
func (l *Logger) Emit(r *models.ValidationResult) error {
	if l == nil || r == nil || l.f == nil {
		return nil
	}
	rec := Record{
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
		Provider:   r.Provider,
		KeySHA256:  HashKey(r.Key),
		IsValid:    r.IsValid,
		StatusCode: r.StatusCode,
		DurationMs: int64(r.ResponseTime * 1000),
	}
	if r.InvalidReason != "" {
		rec.ErrorCode = r.InvalidReason
	} else if !r.IsValid && r.ErrorMessage != "" {
		rec.ErrorCode = truncate(r.ErrorMessage, 80)
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.enc.Encode(&rec); err != nil {
		return err
	}
	if !l.batched {
		return l.f.Sync()
	}
	return nil
}

// Close flushes and closes the underlying file. Idempotent.
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

// Path returns the on-disk path of the audit log (or "" for the no-op logger).
func (l *Logger) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
