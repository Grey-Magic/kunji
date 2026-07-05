package cmd

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"sort"
	"strings"
)

// loadKeys reads newline-separated keys from a path (or stdin if path is "-")
// and returns them trimmed, skipping blanks and `#` comment lines.
func loadKeys(path string) ([]string, error) {
	var r io.Reader
	if path == "" || path == "-" {
		r = os.Stdin
	} else {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		r = f
	}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	var out []string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

// dedupeKeys returns the input keys with duplicates removed, preserving the
// order of first occurrence. duplicates is the number of entries removed.
func dedupeKeys(keys []string) (unique []string, duplicates int) {
	seen := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		if _, ok := seen[k]; ok {
			duplicates++
			continue
		}
		seen[k] = struct{}{}
		unique = append(unique, k)
	}
	return
}

// keyHash produces a stable short hash for a key (used to compare keys across
// runs without exposing the secret itself).
func keyHash(k string) string {
	sum := sha256.Sum256([]byte(k))
	return hex.EncodeToString(sum[:8])
}

// loadValidationResults reads a JSONL file produced by `--format jsonl` and
// returns the parsed ValidationResults keyed by an internal id (provider:hash).
// Lines that fail to parse are skipped silently; mixed-mode files (json, csv)
// are not supported here.
func loadValidationResults(path string) (map[string]string, []string, error) {
	var (
		results = make(map[string]string) // id -> key (so we can hash without re-reading)
		order   []string
	)
	var r io.Reader
	if path == "" || path == "-" {
		r = os.Stdin
	} else {
		f, err := os.Open(path)
		if err != nil {
			return nil, nil, err
		}
		defer f.Close()
		r = f
	}
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		// Cheap parse: split out the key field. We avoid importing the
		// models package here so this stays lightweight and tolerant.
		key := extractJSONField(line, "key")
		valid := extractJSONField(line, "is_valid")
		if key == "" {
			continue
		}
		id := keyHash(key)
		results[id] = valid
		order = append(order, id)
	}
	return results, order, scanner.Err()
}

// extractJSONField pulls the value of a single string/bool field from a JSON
// object line. We hand-parse to avoid loading the whole document into the
// models package; values containing escaped quotes are not handled.
func extractJSONField(line, field string) string {
	key := `"` + field + `":`
	i := strings.Index(line, key)
	if i < 0 {
		return ""
	}
	rest := strings.TrimSpace(line[i+len(key):])
	if rest == "" {
		return ""
	}
	// String value
	if rest[0] == '"' {
		end := strings.IndexByte(rest[1:], '"')
		if end < 0 {
			return ""
		}
		return rest[1 : 1+end]
	}
	// Bool / number — read until comma or closing brace.
	for j := 0; j < len(rest); j++ {
		if rest[j] == ',' || rest[j] == '}' {
			return strings.TrimSpace(rest[:j])
		}
	}
	return strings.TrimSpace(rest)
}

// diffResults compares two JSONL result sets keyed by key-hash and returns
// three slices: ids whose validity changed, ids only in A, and ids only in B.
// ids-only-in-A and ids-only-in-B are unsorted; the caller can sort.
func diffResults(a, b map[string]string) (changed, onlyA, onlyB []string) {
	for id, va := range a {
		vb, ok := b[id]
		if !ok {
			onlyA = append(onlyA, id)
			continue
		}
		if va != vb {
			changed = append(changed, id)
		}
	}
	for id := range b {
		if _, ok := a[id]; !ok {
			onlyB = append(onlyB, id)
		}
	}
	sort.Strings(changed)
	sort.Strings(onlyA)
	sort.Strings(onlyB)
	return
}
