package sink

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"encoding/hex"

	"github.com/Grey-Magic/kunji/pkg/models"
)

// Filter selects which events are emitted to a sink.
type Filter int

const (
	FilterAll     Filter = iota // all results
	FilterValid                 // only successful validations
	FilterInvalid               // only unsuccessful validations (including rate-limited / skipped)
)

func (f Filter) accepts(r *models.ValidationResult) bool {
	switch f {
	case FilterValid:
		return r.IsValid
	case FilterInvalid:
		return !r.IsValid
	default:
		return true
	}
}

// Sink is the contract every result sink must satisfy.
type Sink interface {
	// Emit sends one validation result. Implementations must be safe for
	// concurrent use by many worker goroutines.
	Emit(ctx context.Context, r *models.ValidationResult) error
	// Close flushes any buffered state and releases resources.
	Close() error
}

// Nop is a no-op sink used as a placeholder when no --webhook/--sink is set.
type Nop struct{}

func (Nop) Emit(context.Context, *models.ValidationResult) error { return nil }
func (Nop) Close() error                                         { return nil }

// HTTPSinkOptions configures an HTTPSink. All fields are optional; zero
// values yield the legacy behavior (raw JSON body, no extra headers, no
// retries, no signature).
type HTTPSinkOptions struct {
	// Headers applied to every request in addition to whatever the
	// formatter sets. User-supplied headers win on conflict.
	Headers map[string]string
	// Retries is the number of additional attempts on transient HTTP
	// failures (network error, 5xx, 429). Total attempts = Retries + 1.
	// Zero means a single attempt.
	Retries int
	// RetryBackoff is the initial wait between retries. The wait doubles
	// each attempt (exponential) up to RetryMaxBackoff.
	RetryBackoff time.Duration
	// RetryMaxBackoff caps the per-attempt wait.
	RetryMaxBackoff time.Duration
	// SignatureSecret enables HMAC-SHA256 signing of the request body.
	// When set, SignatureHeader (default "X-Kunji-Signature") is set to
	// "<prefix><hex-digest>". Prefix defaults to empty.
	SignatureSecret string
	SignatureHeader string
	SignaturePrefix string
	// Per-request timeout applied to the underlying HTTP client.
	Timeout time.Duration
}

// HTTPSink POSTs each result to a single URL or a per-provider template URL.
// The body is produced by a Formatter; raw JSON is the default. Every request
// includes the formatter's headers plus any user-supplied headers and an
// optional HMAC signature. Transient failures are retried with exponential
// backoff.
type HTTPSink struct {
	client    *http.Client
	baseURL   string
	filter    Filter
	formatter Formatter
	opts      HTTPSinkOptions
	mu        sync.Mutex
	count     int64
	failCount int64
}

// NewHTTPSink builds an HTTP sink with raw-JSON formatting and a 10-second
// per-request timeout. timeoutSecs<1 falls back to 10s. For platform-specific
// formatting use NewHTTPSinkWithFormatter.
func NewHTTPSink(rawURL string, filter Filter, timeoutSecs int) (*HTTPSink, error) {
	return NewHTTPSinkWithFormatter(rawURL, filter, PlatformRaw, FormatterOptions{}, HTTPSinkOptions{
		Timeout:         timeoutOrDefault(timeoutSecs),
		RetryBackoff:    500 * time.Millisecond,
		RetryMaxBackoff: 10 * time.Second,
	})
}

// NewHTTPSinkWithFormatter builds an HTTP sink with the given platform
// formatter and options. timeoutSecs in opts.Timeout is overridden by a
// non-zero Timeout field.
func NewHTTPSinkWithFormatter(rawURL string, filter Filter, platform Platform, fmtOpts FormatterOptions, sinkOpts HTTPSinkOptions) (*HTTPSink, error) {
	if rawURL == "" {
		return nil, fmt.Errorf("http sink: empty URL")
	}
	if _, err := url.Parse(rawURL); err != nil {
		return nil, fmt.Errorf("http sink: invalid URL %q: %w", rawURL, err)
	}
	formatter, err := buildFormatter(platform, fmtOpts)
	if err != nil {
		return nil, err
	}
	if sinkOpts.Timeout <= 0 {
		sinkOpts.Timeout = 10 * time.Second
	}
	if sinkOpts.RetryBackoff <= 0 {
		sinkOpts.RetryBackoff = 500 * time.Millisecond
	}
	if sinkOpts.RetryMaxBackoff <= 0 {
		sinkOpts.RetryMaxBackoff = 10 * time.Second
	}
	if sinkOpts.SignatureHeader == "" {
		sinkOpts.SignatureHeader = "X-Kunji-Signature"
	}
	return &HTTPSink{
		client:    &http.Client{Timeout: sinkOpts.Timeout},
		baseURL:   rawURL,
		filter:    filter,
		formatter: formatter,
		opts:      sinkOpts,
	}, nil
}

func timeoutOrDefault(secs int) time.Duration {
	if secs < 1 {
		return 10 * time.Second
	}
	return time.Duration(secs) * time.Second
}

func (s *HTTPSink) resolveURL(provider string) string {
	if !strings.Contains(s.baseURL, "{provider}") {
		return s.baseURL
	}
	// Provider names go into URL path segments; replace anything that
	// isn't a URL-safe character with underscore.
	safe := make([]byte, 0, len(provider))
	for i := 0; i < len(provider); i++ {
		c := provider[i]
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			c == '-' || c == '_':
			safe = append(safe, c)
		default:
			safe = append(safe, '_')
		}
	}
	return strings.ReplaceAll(s.baseURL, "{provider}", string(safe))
}

// Emit POSTs the result, retrying transient failures.
func (s *HTTPSink) Emit(ctx context.Context, r *models.ValidationResult) error {
	if r == nil || !s.filter.accepts(r) {
		return nil
	}

	body, fmtHeaders, err := s.formatter.Format(r)
	if err != nil {
		s.recordFailure()
		return fmt.Errorf("http sink: format: %w", err)
	}

	sigName, sigValue := SignHMAC(body, s.opts.SignatureSecret, s.opts.SignatureHeader, s.opts.SignaturePrefix)
	headers := mergeHeaders(fmtHeaders, s.opts.Headers, sigName, sigValue)

	endpoint := s.resolveURL(r.Provider)

	attempts := s.opts.Retries + 1
	backoff := s.opts.RetryBackoff
	var lastErr error
	for i := 0; i < attempts; i++ {
		err := s.postOnce(ctx, endpoint, body, headers)
		if err == nil {
			s.recordSuccess()
			return nil
		}
		lastErr = err
		if i == attempts-1 || !isRetryable(err) {
			break
		}
		select {
		case <-ctx.Done():
			s.recordFailure()
			return ctx.Err()
		case <-time.After(backoff):
		}
		backoff *= 2
		if backoff > s.opts.RetryMaxBackoff {
			backoff = s.opts.RetryMaxBackoff
		}
	}
	s.recordFailure()
	return fmt.Errorf("http sink: post %s failed after %d attempt(s): %w", endpoint, attempts, lastErr)
}

func (s *HTTPSink) postOnce(ctx context.Context, endpoint string, body []byte, headers map[string]string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return &permanentError{err: fmt.Errorf("build request: %w", err)}
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if req.Header.Get("User-Agent") == "" {
		req.Header.Set("User-Agent", "kunji-sink/1.0")
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return err // network errors are retryable
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return nil
	case resp.StatusCode == 429 || resp.StatusCode >= 500:
		return &transientError{status: resp.StatusCode}
	default:
		return &permanentError{err: fmt.Errorf("status %d", resp.StatusCode)}
	}
}

func (s *HTTPSink) recordSuccess() {
	s.mu.Lock()
	s.count++
	s.mu.Unlock()
}

func (s *HTTPSink) recordFailure() {
	s.mu.Lock()
	s.count++
	s.failCount++
	s.mu.Unlock()
}

// Stats returns the count of attempted and failed emits since construction.
func (s *HTTPSink) Stats() (sent int64, failed int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count, s.failCount
}

// Platform returns the formatter platform name.
func (s *HTTPSink) Platform() string {
	return string(s.formatter.Platform())
}

func (s *HTTPSink) Close() error { return nil }

// ---------- error classification for retry ----------

type transientError struct{ status int }

func (e *transientError) Error() string {
	return fmt.Sprintf("transient: status %d", e.status)
}

type permanentError struct{ err error }

func (e *permanentError) Error() string { return "permanent: " + e.err.Error() }
func (e *permanentError) Unwrap() error { return e.err }

func isRetryable(err error) bool {
	var t *transientError
	if errors.As(err, &t) {
		return true
	}
	// Any non-permanent error (network, timeout, EOF) is retryable.
	var p *permanentError
	if errors.As(err, &p) {
		return false
	}
	return true
}

// Sentinel for tests / callers that want to distinguish.
var ErrWebhookAllRetriesFailed = errors.New("webhook: all retries failed")

// FileSink writes one JSON file per result into a directory. Filenames are
// derived from a hash of (provider, key) plus a millisecond timestamp so that
// re-runs of the same key are idempotent (overwrite the same path).
type FileSink struct {
	dir    string
	filter Filter
	mu     sync.Mutex
	count  int64
}

// NewFileSink prepares a directory for output, creating it if missing.
func NewFileSink(dir string, filter Filter) (*FileSink, error) {
	if dir == "" {
		return nil, fmt.Errorf("file sink: empty directory")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("file sink: mkdir %s: %w", dir, err)
	}
	return &FileSink{dir: dir, filter: filter}, nil
}

func (s *FileSink) Emit(ctx context.Context, r *models.ValidationResult) error {
	if r == nil || !s.filter.accepts(r) {
		return nil
	}
	body, err := jsonMarshal(r)
	if err != nil {
		return fmt.Errorf("file sink: marshal: %w", err)
	}
	sum := sha256Sum([]byte(r.Provider + "|" + r.Key))
	name := fmt.Sprintf("%d-%s-%s.json",
		time.Now().UnixMilli(),
		sanitize(r.Provider),
		hex.EncodeToString(sum[:6]),
	)
	dest := filepath.Join(s.dir, name)
	if err := os.WriteFile(dest, body, 0o600); err != nil {
		return fmt.Errorf("file sink: write %s: %w", dest, err)
	}
	s.mu.Lock()
	s.count++
	s.mu.Unlock()
	return nil
}

// Count returns the number of files emitted since construction.
func (s *FileSink) Count() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.count
}

func (s *FileSink) Close() error { return nil }
