package source

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
)

// Source produces a stream of keys to validate. Implementations must be safe
// to call Next() concurrently; the validate pipeline pulls from a single
// goroutine so concurrency is not strictly required.
type Source interface {
	// Name returns the kind identifier (e.g. "file", "stdin").
	Name() string
	// Next returns the next key. io.EOF signals end-of-stream.
	Next(ctx context.Context) (string, error)
	// Total reports an estimate of total keys if known, or 0 if streaming.
	Total() int
	// Close releases any underlying resources.
	Close() error
}

// Registry maps kind identifiers to factory functions. Built-ins are
// registered in init(); external code can register additional kinds before
// invoking the validate command.
type Registry struct {
	factories map[string]func(args []string) (Source, error)
}

func NewRegistry() *Registry {
	return &Registry{factories: make(map[string]func(args []string) (Source, error))}
}

// Register installs a factory under the given kind identifier. Empty kind or
// nil factory panics — these are programming errors, not runtime conditions.
func (r *Registry) Register(kind string, factory func(args []string) (Source, error)) {
	if kind == "" || factory == nil {
		panic("source: Register called with empty kind or nil factory")
	}
	if _, exists := r.factories[kind]; exists {
		panic(fmt.Sprintf("source: kind %q already registered", kind))
	}
	r.factories[kind] = factory
}

// Open constructs a source by kind with the provided args. The first arg is
// treated as the primary argument (path, URL, etc.); subsequent args are
// plugin-specific.
func (r *Registry) Open(kind string, args []string) (Source, error) {
	f, ok := r.factories[kind]
	if !ok {
		return nil, fmt.Errorf("unknown source kind %q (available: %s)", kind, strings.Join(r.Kinds(), ", "))
	}
	return f(args)
}

// Kinds returns the registered kind identifiers in sorted order, for use in
// help text and error messages.
func (r *Registry) Kinds() []string {
	out := make([]string, 0, len(r.factories))
	for k := range r.factories {
		out = append(out, k)
	}
	// simple sort; tiny slice
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j-1] > out[j]; j-- {
			out[j-1], out[j] = out[j], out[j-1]
		}
	}
	return out
}

// Default is the registry with built-in sources registered.
var Default = NewRegistry()

func init() {
	Default.Register("file", newFileSource)
	Default.Register("stdin", newStdinSource)
}

// fileSource reads newline-separated keys from a path.
type fileSource struct {
	scanner *bufio.Scanner
	file    *os.File
	total   int
}

func newFileSource(args []string) (Source, error) {
	if len(args) == 0 || args[0] == "" {
		return nil, fmt.Errorf("file source: requires a path argument (use '-' for stdin)")
	}
	path := args[0]
	var (
		f     *os.File
		err   error
		rdr   io.Reader
		total = 0
	)
	if path == "-" {
		rdr = os.Stdin
	} else {
		f, err = os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("file source: open %s: %w", path, err)
		}
		rdr = f
	}
	scanner := bufio.NewScanner(rdr)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	// Count non-blank, non-comment lines up front so Total() is accurate.
	// We re-open the file because we've already buffered the reader.
	if path != "-" {
		f.Close()
		counting, err := os.Open(path)
		if err == nil {
			defer counting.Close()
			c := bufio.NewScanner(counting)
			c.Buffer(make([]byte, 64*1024), 16*1024*1024)
			for c.Scan() {
				line := strings.TrimSpace(c.Text())
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				total++
			}
		}
		f, err = os.Open(path)
		if err != nil {
			return nil, fmt.Errorf("file source: reopen %s: %w", path, err)
		}
		rdr = f
		scanner = bufio.NewScanner(rdr)
		scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	}
	return &fileSource{scanner: scanner, file: f, total: total}, nil
}

func (s *fileSource) Name() string { return "file" }
func (s *fileSource) Total() int   { return s.total }
func (s *fileSource) Close() error {
	if s.file != nil {
		return s.file.Close()
	}
	return nil
}
func (s *fileSource) Next(ctx context.Context) (string, error) {
	for s.scanner.Scan() {
		line := strings.TrimSpace(s.scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return line, nil
	}
	if err := s.scanner.Err(); err != nil {
		return "", err
	}
	return "", io.EOF
}

// stdinSource reads newline-separated keys from os.Stdin.
type stdinSource struct {
	scanner *bufio.Scanner
}

func newStdinSource(args []string) (Source, error) {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	return &stdinSource{scanner: scanner}, nil
}

func (s *stdinSource) Name() string { return "stdin" }
func (s *stdinSource) Total() int   { return 0 } // streaming; unknown
func (s *stdinSource) Close() error { return nil }
func (s *stdinSource) Next(ctx context.Context) (string, error) {
	for s.scanner.Scan() {
		line := strings.TrimSpace(s.scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		return line, nil
	}
	if err := s.scanner.Err(); err != nil {
		return "", err
	}
	return "", io.EOF
}

// SliceSource is a small adapter for tests and for callers that already have
// keys in memory. Not registered by default.
type SliceSource struct {
	keys  []string
	index int
}

func NewSliceSource(keys ...string) *SliceSource {
	return &SliceSource{keys: append([]string(nil), keys...)}
}

func (s *SliceSource) Name() string { return "slice" }
func (s *SliceSource) Total() int   { return len(s.keys) }
func (s *SliceSource) Close() error { return nil }
func (s *SliceSource) Next(ctx context.Context) (string, error) {
	if s.index >= len(s.keys) {
		return "", io.EOF
	}
	k := s.keys[s.index]
	s.index++
	return k, nil
}
