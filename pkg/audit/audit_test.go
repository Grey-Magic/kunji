package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Grey-Magic/kunji/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHashKey_StableAndDistinct(t *testing.T) {
	a := HashKey("sk-abc")
	b := HashKey("sk-abc")
	c := HashKey("sk-xyz")

	assert.Equal(t, a, b)
	assert.NotEqual(t, a, c)
	assert.Len(t, a, 64, "SHA-256 hex is 64 chars")
}

func TestLogger_EmptyPathIsNoop(t *testing.T) {
	l, err := Open("")
	require.NoError(t, err)
	assert.NoError(t, l.Emit(&models.ValidationResult{Key: "x", Provider: "p", IsValid: true}))
	assert.NoError(t, l.Close())
	assert.Empty(t, l.Path())
}

func TestLogger_AppendsOneLinePerEmit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")

	l, err := Open(path)
	require.NoError(t, err)

	res1 := &models.ValidationResult{Key: "sk-aaa", Provider: "openai", IsValid: true, StatusCode: 200, ResponseTime: 0.123}
	res2 := &models.ValidationResult{Key: "sk-bbb", Provider: "stripe", IsValid: false, StatusCode: 401, ResponseTime: 0.05, InvalidReason: "INVALID_KEY"}

	require.NoError(t, l.Emit(res1))
	require.NoError(t, l.Emit(res2))
	require.NoError(t, l.Close())

	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()

	scanner := bufio.NewScanner(f)
	cnt := 0
	for scanner.Scan() {
		var rec Record
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &rec))
		switch cnt {
		case 0:
			assert.Equal(t, "openai", rec.Provider)
			assert.True(t, rec.IsValid)
			assert.Equal(t, 200, rec.StatusCode)
			assert.NotEqual(t, "", rec.KeySHA256)
			assert.NotEqual(t, "sk-aaa", rec.KeySHA256, "raw key must never appear")
			assert.Equal(t, HashKey("sk-aaa"), rec.KeySHA256)
		case 1:
			assert.Equal(t, "stripe", rec.Provider)
			assert.False(t, rec.IsValid)
			assert.Equal(t, 401, rec.StatusCode)
			assert.Equal(t, "INVALID_KEY", rec.ErrorCode)
			assert.Equal(t, HashKey("sk-bbb"), rec.KeySHA256)
		}
		cnt++
	}
	assert.Equal(t, 2, cnt)
}

func TestLogger_ConcurrentEmitsAreSerialized(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "audit.jsonl")

	l, err := Open(path)
	require.NoError(t, err)

	var wg sync.WaitGroup
	const N = 50
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_ = l.Emit(&models.ValidationResult{Key: "k", Provider: "p", IsValid: true, StatusCode: 200})
		}(i)
	}
	wg.Wait()
	require.NoError(t, l.Close())

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	assert.Equal(t, N, lines, "every Emit must produce exactly one newline-terminated line")
}

func TestDefaultPath(t *testing.T) {
	t.Setenv("KUNJI_AUDIT_FILE", "")
	p := DefaultPath()
	assert.NotEmpty(t, p)
	assert.Contains(t, p, ".kunji")
	assert.Contains(t, p, "audit.jsonl")
}
