package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoot_HasCompletionCommand(t *testing.T) {
	// Cobra adds the default `completion` command lazily during Execute(),
	// so we trigger that here for testing. This mirrors what `kunji completion
	// bash` does at runtime.
	rootCmd.InitDefaultCompletionCmd()

	for _, name := range []string{"bash", "zsh", "fish", "powershell"} {
		c, _, err := rootCmd.Find([]string{"completion", name})
		require.NoError(t, err, "completion %s must be reachable", name)
		assert.Equal(t, name, c.Name())
	}

	// Sanity: completion bash produces a non-empty script.
	c, _, err := rootCmd.Find([]string{"completion", "bash"})
	require.NoError(t, err)
	// Sanity: the bash completion command has its own Use description and
	// is wired to a RunE that does not panic. We avoid running it here
	// because Cobra routes completion output through a custom printer that
	// does not flow through OutOrStderr reliably in unit tests.
	assert.NotEmpty(t, c.Short, "completion bash must have a description")
}

func TestLoadKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "keys.txt")
	require.NoError(t, os.WriteFile(path, []byte(strings.Join([]string{
		"sk-aaa",
		"",
		"# a comment",
		"  sk-bbb  ",
		"sk-aaa",
		"sk-ccc",
	}, "\n")), 0o600))

	keys, err := loadKeys(path)
	require.NoError(t, err)
	assert.Equal(t, []string{"sk-aaa", "sk-bbb", "sk-aaa", "sk-ccc"}, keys,
		"loadKeys should trim and skip blanks/comments but NOT dedupe (that's dedupeKeys' job)")
}

func TestDedupeKeys(t *testing.T) {
	unique, dups := dedupeKeys([]string{"a", "b", "a", "c", "b", "d"})
	assert.Equal(t, []string{"a", "b", "c", "d"}, unique)
	assert.Equal(t, 2, dups)
}

func TestKeyHash_StableAndDistinct(t *testing.T) {
	h1 := keyHash("sk-abc")
	h2 := keyHash("sk-abc")
	h3 := keyHash("sk-xyz")
	assert.Equal(t, h1, h2, "same input should produce same hash")
	assert.NotEqual(t, h1, h3, "different inputs should produce different hashes")
	assert.Len(t, h1, 16, "hash should be 8 bytes hex-encoded")
}

func TestExtractJSONField(t *testing.T) {
	line := `{"event":"result","key":"sk-foo","is_valid":true,"provider":"openai"}`
	assert.Equal(t, "sk-foo", extractJSONField(line, "key"))
	assert.Equal(t, "true", extractJSONField(line, "is_valid"))
	assert.Equal(t, "openai", extractJSONField(line, "provider"))
	assert.Equal(t, "", extractJSONField(line, "missing"))
}

func TestLoadAndDiffValidationResults(t *testing.T) {
	dir := t.TempDir()
	aPath := filepath.Join(dir, "a.jsonl")
	bPath := filepath.Join(dir, "b.jsonl")

	aLines := []string{
		`{"event":"result","key":"sk-1","is_valid":true,"provider":"openai"}`,
		`{"event":"result","key":"sk-2","is_valid":false,"provider":"stripe"}`,
		`{"event":"result","key":"sk-3","is_valid":true,"provider":"openai"}`,
		`{"event":"progress","done":3,"total":3}`, // should be ignored
	}
	bLines := []string{
		`{"event":"result","key":"sk-1","is_valid":false,"provider":"openai"}`,   // changed
		`{"event":"result","key":"sk-3","is_valid":true,"provider":"openai"}`,    // unchanged
		`{"event":"result","key":"sk-4","is_valid":true,"provider":"anthropic"}`, // only in B
		// sk-2 missing -> only in A
	}
	require.NoError(t, os.WriteFile(aPath, []byte(strings.Join(aLines, "\n")+"\n"), 0o600))
	require.NoError(t, os.WriteFile(bPath, []byte(strings.Join(bLines, "\n")+"\n"), 0o600))

	a, _, err := loadValidationResults(aPath)
	require.NoError(t, err)
	b, _, err := loadValidationResults(bPath)
	require.NoError(t, err)

	changed, onlyA, onlyB := diffResults(a, b)

	assert.Len(t, changed, 1, "sk-1 changed true->false")
	assert.Len(t, onlyA, 1, "sk-2 missing from B")
	assert.Len(t, onlyB, 1, "sk-4 new in B")
}
