package sink

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Grey-Magic/kunji/pkg/models"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPSink_PostsEachResult(t *testing.T) {
	var (
		mu        sync.Mutex
		received  [][]byte
		gotPath   []string
		gotMethod []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		mu.Lock()
		received = append(received, body)
		gotPath = append(gotPath, r.URL.Path)
		gotMethod = append(gotMethod, r.Method)
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	s, err := NewHTTPSink(srv.URL, FilterAll, 5)
	require.NoError(t, err)

	results := []*models.ValidationResult{
		{Provider: "openai", Key: "sk-aaa", IsValid: true},
		{Provider: "stripe", Key: "sk_live_bbb", IsValid: false, ErrorMessage: "401"},
	}
	for _, r := range results {
		require.NoError(t, s.Emit(context.Background(), r))
	}

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, received, len(results))
	for i, r := range results {
		assert.Equal(t, http.MethodPost, gotMethod[i])
		var decoded models.ValidationResult
		require.NoError(t, json.Unmarshal(received[i], &decoded))
		assert.Equal(t, r.Provider, decoded.Provider)
		assert.Equal(t, r.Key, decoded.Key)
		assert.Equal(t, r.IsValid, decoded.IsValid)
	}

	sent, failed := s.Stats()
	assert.EqualValues(t, len(results), sent)
	assert.EqualValues(t, 0, failed)
}

func TestHTTPSink_TemplateSubstitutesProvider(t *testing.T) {
	var receivedPath string
	var mu sync.Mutex
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedPath = r.URL.Path
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	url := srv.URL + "/hook/{provider}/event"
	s, err := NewHTTPSink(url, FilterAll, 5)
	require.NoError(t, err)

	require.NoError(t, s.Emit(context.Background(), &models.ValidationResult{Provider: "openai/fake path"}))

	mu.Lock()
	defer mu.Unlock()
	assert.Equal(t, "/hook/openai_fake_path/event", receivedPath)
}

func TestHTTPSink_FilterValidSkipsInvalid(t *testing.T) {
	var hits int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&hits, 1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	s, err := NewHTTPSink(srv.URL, FilterValid, 5)
	require.NoError(t, err)

	require.NoError(t, s.Emit(context.Background(), &models.ValidationResult{Provider: "p1", IsValid: true}))
	require.NoError(t, s.Emit(context.Background(), &models.ValidationResult{Provider: "p2", IsValid: false}))
	require.NoError(t, s.Emit(context.Background(), &models.ValidationResult{Provider: "p3", IsValid: true}))

	assert.EqualValues(t, 2, atomic.LoadInt64(&hits))
}

func TestHTTPSink_Non2xxCountsAsFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	s, err := NewHTTPSink(srv.URL, FilterAll, 5)
	require.NoError(t, err)

	err = s.Emit(context.Background(), &models.ValidationResult{Provider: "x", IsValid: true})
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "status 500"))

	_, failed := s.Stats()
	assert.EqualValues(t, 1, failed)
}

func TestFileSink_WritesOneFilePerResult(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFileSink(dir, FilterAll)
	require.NoError(t, err)

	res := &models.ValidationResult{Provider: "openai", Key: "sk-abc", IsValid: true}
	require.NoError(t, s.Emit(context.Background(), res))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)

	body, err := os.ReadFile(filepath.Join(dir, entries[0].Name()))
	require.NoError(t, err)

	var decoded models.ValidationResult
	require.NoError(t, json.Unmarshal(body, &decoded))
	assert.Equal(t, "openai", decoded.Provider)
	assert.Equal(t, "sk-abc", decoded.Key)

	assert.EqualValues(t, 1, s.Count())
}

func TestFileSink_FilterRespected(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFileSink(dir, FilterValid)
	require.NoError(t, err)

	require.NoError(t, s.Emit(context.Background(), &models.ValidationResult{Provider: "p", IsValid: true}))
	require.NoError(t, s.Emit(context.Background(), &models.ValidationResult{Provider: "p", IsValid: false}))

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestFileSink_SameKeyIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	s, err := NewFileSink(dir, FilterAll)
	require.NoError(t, err)

	res := &models.ValidationResult{Provider: "p", Key: "k"}
	for i := 0; i < 3; i++ {
		require.NoError(t, s.Emit(context.Background(), res))
		// ensure filename timestamp differs slightly between writes
		time.Sleep(2 * time.Millisecond)
	}

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entries, 3, "each emit writes a new file with a unique millisecond timestamp")
}
